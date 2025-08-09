package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"math/big"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"
)

const (
	FlagSyn    uint8 = 0x02
	FlagAck    uint8 = 0x10
	FlagSynAck uint8 = FlagSyn + FlagAck
)

func main() {
	// 引数を取得
	destinationHost, destinationPort, maxExecutionCount := getArguments()

	// macOSでのフォールバック実行
	if runtime.GOOS == "darwin" && os.Geteuid() != 0 {
		fmt.Println("macOSで非root権限で実行中: TCP接続テストモードを使用します")
		runTcpConnectionTest(destinationHost, destinationPort, maxExecutionCount)
		return
	}
	
	// macOSでroot権限があってもrawソケットの問題がある場合のフォールバック
	if runtime.GOOS == "darwin" && os.Geteuid() == 0 {
		fmt.Println("警告: macOSではrawソケットの制限により、TCP接続テストモードを推奨します")
		fmt.Println("rawソケットモードを続行する場合は継続し、問題が発生した場合は非root権限で再実行してください")
	}

	// root権限チェック
	if os.Geteuid() != 0 {
		fmt.Println("エラー: このプログラムはroot権限で実行する必要があります")
		fmt.Println("sudo ./synpack -h <ホスト> -p <ポート> -c <回数> で実行してください")
		fmt.Println("または、macOSでは非root権限でTCP接続テストモードが利用可能です")
		os.Exit(1)
	}

	// シグナル受信設定
	signalChan := make(chan os.Signal, 1)
	// [ctrl+c]をキャッチする
	signal.Notify(signalChan, os.Interrupt)

	// ローカルIPアドレス
	localInterfaceName, localIpAddress := getLocalInterface()
	if localInterfaceName == "" || localIpAddress == "" {
		fmt.Println("ローカルで有効なインタフェースを取得できません")
		os.Exit(1)
	}

	// 送信元インターフェース名
	sourceInterfaceName := localInterfaceName

	// 送信元IPアドレス
	sourceIpAddress := net.ParseIP(localIpAddress)

	// 送信先ホスト名から送信先IPアドレスを取得
	targetIpAddress := getTargetIpAddress(destinationHost)
	if targetIpAddress == "" {
		fmt.Println("送信先ホストに対応するIPアドレスを取得できません")
		os.Exit(1)
	}
	// 送信先IPアドレス
	destinationIpAddress := net.ParseIP(targetIpAddress)
	// 実行結果を記録
	var rtts []time.Duration

	shouldExit := false
	go func() {
		<-signalChan // ctrl+cが押されるまで待機
		shouldExit = true
	}()

	fmt.Printf(
		"Synpack %s (%s) -> %s (%s)\n",
		sourceInterfaceName,
		sourceIpAddress,
		destinationHost,
		destinationIpAddress,
	)
	executedCount := 0
	successReceivedCount := 0
	// 引数で指定された実行回数を超えるまで処理を繰り返す
	for executedCount < maxExecutionCount {
		// ctrl+cが押されたら処理を停止する
		if shouldExit {
			break
		}

		// 送信元ポート番号（毎実行時にポート番号を変える）
		sourcePort := generateAvailablePort(sourceIpAddress)
		if sourcePort == 0 {
			fmt.Println("ローカルで有効なポート番号を取得できません")
			os.Exit(1)
		}

		// シーケンス番号をランダムに取得
		n, err := rand.Int(rand.Reader, big.NewInt(1<<32))
		if err != nil {
			fmt.Println("シーケンス番号の採番に失敗しました")
			os.Exit(1)
		}
		seqNumber := uint32(n.Int64())

		// 送受信用のソケットを作成
		socket, err := createSocket()
		if err != nil {
			fmt.Printf("unix.Socket実行時にエラーが発生しました: %v\n", err)
			fmt.Println("macOSでは「システム環境設定 > セキュリティとプライバシー > プライバシー > フルディスクアクセス」でターミナルを許可する必要がある場合があります")
			os.Exit(1)
		}

		// SYNパケットを生成
		packet := createSynPacket(
			sourceIpAddress,
			destinationIpAddress,
			sourcePort,
			destinationPort,
			seqNumber,
		)

		// 送信元アドレス設定
		sourceSocketAddress := getSocketAddress(sourceIpAddress, sourcePort)
		err = bindSocketAddress(socket, sourceSocketAddress)
		if err != nil {
			fmt.Println("unix.Bind実行時にエラーが発生しました", err)
			os.Exit(1)
		}

		// 送信先アドレス設定
		destinationSocketAddress := getSocketAddress(destinationIpAddress, destinationPort)

		start := time.Now()
		// パケットを送信
		err = sendPacket(socket, packet, destinationSocketAddress)
		if err != nil {
			fmt.Printf("unix.Sendto実行時にエラーが発生しました: %v\n", err)
			os.Exit(1)
		}
		
		// macOS用デバッグ情報
		if runtime.GOOS == "darwin" {
			fmt.Printf("デバッグ: パケット送信完了 (len=%d, src=%s:%d -> dst=%s:%d)\n",
				len(packet), sourceIpAddress, sourcePort, destinationIpAddress, destinationPort)
		}

		buf := make([]byte, 4096)

		// タイムアウトの設定(1秒)
		timeout := time.Now().Add(1 * time.Second)
		receivedPacketCount := 0
		for time.Now().Before(timeout) {
			// パケットを受信
			err = receivePacket(socket, buf)
			if err != nil {
				// タイムアウトまたは受信エラーをチェック
				if strings.Contains(err.Error(), "resource temporarily unavailable") {
					// タイムアウト（正常）なのでリトライ
					continue
				}
				// macOS用デバッグ情報
				if runtime.GOOS == "darwin" {
					fmt.Printf("デバッグ: 受信エラー - %v\n", err)
				}
				continue
			}
			
			receivedPacketCount++
			// macOS用デバッグ情報
			if runtime.GOOS == "darwin" {
				fmt.Printf("デバッグ: パケット受信 #%d (len=%d bytes)\n", receivedPacketCount, len(buf))
			}

			// 受信データ長をチェック
			receivedLen := 0
			for i, b := range buf {
				if b == 0 {
					receivedLen = i
					break
				}
			}
			if receivedLen == 0 {
				receivedLen = len(buf)
			}
			
			if receivedLen < 40 {
				// パケットが短すぎるのでリトライ
				continue
			}

			tcpHeader := buf[20:40]
			receivedSourceIpAddress := net.IP(buf[12:16])
			receivedDestinationIpAddress := net.IP(buf[16:20])
			receivedSourcePort := int(binary.BigEndian.Uint16(tcpHeader[0:2]))
			receivedDestinationPort := int(binary.BigEndian.Uint16(tcpHeader[2:4]))

			// Synフラグ送信時の送信元IPアドレスとパケット受信時の送信先IPアドレスを比較
			// Synフラグ送信時の送信先IPアドレスとパケット受信時の送信元IPアドレスを比較
			// Synフラグ送信時の送信元ポートとパケット受信時の送信先ポートを比較
			// Synフラグ送信時の送信先ポートとパケット受信時の送信元ポートを比較
			if !receivedDestinationIpAddress.Equal(sourceIpAddress) ||
				!receivedSourceIpAddress.Equal(destinationIpAddress) ||
				sourcePort != receivedDestinationPort ||
				destinationPort != receivedSourcePort {
				// 不一致の場合は関係ないパケットなのでリトライ
				continue
			}

			// 受信パケットを解析
			err = parsePacket(tcpHeader, seqNumber)
			if err == nil {
				// SYN-ACK確認成功
				successReceivedCount++
				rtt := time.Since(start)
				rtts = append(rtts, rtt)
				fmt.Printf(
					"len=%d ip=%s port=%d seq=%d rtt=%.2f ms\n",
					receivedLen,
					destinationIpAddress,
					destinationPort,
					seqNumber,
					float64(rtt.Microseconds())/1000,
				)
				// SYN-ACK確認後に不要な受信処理を行わないようにループを抜ける
				break
			}
		}
		
		// ソケットをクローズ
		unix.Close(socket)
		
		// 送信先に負荷を掛けないように次の実行まで待機
		time.Sleep(1 * time.Second)

		// 実行回数をインクリメント
		executedCount++
	}

	// RTT結果を表示
	fmt.Printf("\n--- %s Synpack statistic ---\n", destinationHost)
	fmt.Printf(
		"%d packets transmitted, %d packets received, %.2f%% packet loss\n",
		executedCount,
		successReceivedCount,
		(float64(executedCount-successReceivedCount)/float64(executedCount))*100,
	)
	if len(rtts) > 0 {
		minRtt, maxRtt, sumRtt := rtts[0], rtts[0], time.Duration(0)
		for _, rtt := range rtts {
			if rtt < minRtt {
				minRtt = rtt
			}
			if rtt > maxRtt {
				maxRtt = rtt
			}
			sumRtt += rtt
		}
		fmt.Printf("round-trip min/avg/max = %.2f/%.2f/%.2f ms\n",
			float64(minRtt.Microseconds())/1000,
			float64(sumRtt.Microseconds())/float64(len(rtts))/1000,
			float64(maxRtt.Microseconds())/1000)
	} else {
		fmt.Printf("round-trip min/avg/max = 0.0/0.0/0.0\n")
	}
}

// SYNパケットを生成
func createSynPacket(
	sourceIpAddress net.IP,
	destinationIpAddress net.IP,
	sourcePort int,
	destinationPort int,
	seqNumber uint32,
) []byte {
	return createTcpHeader(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber)
}

// TCPヘッダーを生成
func createTcpHeader(
	sourceIpAddress net.IP,
	destinationIpAddress net.IP,
	sourcePort int,
	destinationPort int,
	seqNumber uint32,
) []byte {
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], uint16(sourcePort))      // 送信元ポート(16ビット)
	binary.BigEndian.PutUint16(header[2:4], uint16(destinationPort)) // 送信先ポート(16ビット)
	binary.BigEndian.PutUint32(header[4:8], seqNumber)               // シーケンス番号(32ビット)
	binary.BigEndian.PutUint32(header[8:12], 0)                      // ACK番号(32ビット)
	header[12] = 0x50                                                // データオフセット(4ビット) + 予約領域(4ビット)
	header[13] = FlagSyn                                             // コントロールビット(8ビット)
	header[14], header[15] = 0x72, 0x10                              // ウィンドウサイズ(16ビット)
	binary.BigEndian.PutUint16(header[18:20], 0)                     // 緊急ポインタ(16ビット)
	// チェックサム計算
	pseudoHeader := createPseudoHeader(sourceIpAddress, destinationIpAddress, header)
	checksum := calcChecksum(pseudoHeader)
	header[16], header[17] = byte(checksum>>8), byte(checksum&0xff) // チェックサム(16ビット)
	return header
}

// 疑似ヘッダーを生成
func createPseudoHeader(
	sourceIpAddress net.IP,
	destinationIpAddress net.IP,
	tcpHeader []byte,
) []byte {
	pseudoHeader := make([]byte, 12+len(tcpHeader))
	copy(pseudoHeader[0:4], sourceIpAddress.To4())
	copy(pseudoHeader[4:8], destinationIpAddress.To4())
	pseudoHeader[8] = 0x00 // 予約
	pseudoHeader[9] = unix.IPPROTO_TCP
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(len(tcpHeader)))
	copy(pseudoHeader[12:], tcpHeader)
	return pseudoHeader
}

// パケットを解析してSYN-ACKを確認
func parsePacket(tcpHeader []byte, seqNumber uint32) error {
	// フラグ
	flags := tcpHeader[13]

	// ACK番号
	ackNumber := binary.BigEndian.Uint32(tcpHeader[8:12]) // 8バイト目から

	if ackNumber == seqNumber+1 && (flags&FlagSynAck) == FlagSynAck {
		// SYN-ACKフラグの受信が成功
		return nil
	}

	return fmt.Errorf("受信したパケットがSYN-ACKではありません")
}

// チェックサム計算
func calcChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// ローカル環境のネットワークインターフェースからインタフェース名とIPアドレスを取得
func getLocalInterface() (string, string) {
	// インターフェースを取得
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}

	for _, iface := range interfaces {
		// 以下のインターフェースの場合はスキップ
		// - 無効状態
		// - ループバックアドレス
		// - インターフェース名がDocker関連
		if (iface.Flags&net.FlagUp) == 0 ||
			(iface.Flags&net.FlagLoopback) != 0 ||
			hasDockerInterfaceName(iface.Name) {
			continue
		}

		// IPアドレス情報を取得
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// IPv4のみを対象
			if ip == nil || ip.IsLoopback() || !ip.IsGlobalUnicast() || ip.To4() == nil {
				continue
			}
			return iface.Name, ip.String()
		}
	}
	return "", ""
}

// Docker関連のインターフェース名か判定する
func hasDockerInterfaceName(name string) bool {
	// Dockerで使われる典型的なインターフェース名
	dockerPrefixes := []string{"docker", "br-", "veth", "tunl", "flannel", "cni"}

	for _, prefix := range dockerPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// 利用可能なポート番号を取得
func generateAvailablePort(sourceIpAddress net.IP) int {
	// 「:0」を指定することでシステムで利用可能なポート番号を返す
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", sourceIpAddress.To4()))
	// LISTEN不可の場合は利用できない（他の処理で利用中のポート）
	if err != nil {
		return 0
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}
	return address.Port
}

func getTargetIpAddress(destinationHost string) string {
	ips, err := net.LookupIP(destinationHost)
	if err != nil {
		return ""
	}

	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

func createSocket() (int, error) {
	// ソケットを作成
	// - アドレスファミリー:IPv4
	// - ソケットの種類:低レベルソケット
	// - プロトコル:TCP
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_TCP)
	return fd, err
}

func sendPacket(fd int, packet []byte, address *unix.SockaddrInet4) error {
	err := unix.Sendto(fd, packet, 0, address)
	return err
}

func receivePacket(fd int, buf []byte) error {
	// ソケットにタイムアウトを設定（100ms）
	err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{
		Sec:  0,
		Usec: 100000, // 100ms
	})
	if err != nil {
		return fmt.Errorf("ソケットタイムアウト設定エラー: %v", err)
	}
	
	_, _, err = unix.Recvfrom(fd, buf, 0)
	return err
}

func getSocketAddress(ipAddress net.IP, port int) *unix.SockaddrInet4 {
	address := unix.SockaddrInet4{
		Port: port,
	}
	copy(address.Addr[:], ipAddress.To4())
	return &address
}

func bindSocketAddress(fd int, address *unix.SockaddrInet4) error {
	err := unix.Bind(fd, address)
	return err
}

func getArguments() (string, int, int) {
	// 引数を取得
	argHost := flag.String("h", "", "送信先のホスト名(必須)")
	argPort := flag.Int("p", 0, "送信先のポート番号(必須:0-65535)")
	argCount := flag.Int("c", 0, "実行回数(必須)")
	flag.Parse()

	// 送信先ホスト名の必須チェック
	if *argHost == "" {
		fmt.Fprintln(os.Stderr, "送信先ホスト名(-h)は必須です")
		flag.Usage()
		os.Exit(1)
	}

	// 送信先ポート番号の妥当性チェック
	if *argPort < 0 || *argPort > 65535 {
		fmt.Fprintln(os.Stderr, "送信先ポート番号(-p)は1〜65535の範囲で指定してください")
		flag.Usage()
		os.Exit(1)
	}

	// 実行回数の妥当性チェック
	if *argCount <= 0 {
		fmt.Fprintln(os.Stderr, "実行回数(-c)は1以上を指定してください")
		flag.Usage()
		os.Exit(1)
	}

	return *argHost, *argPort, *argCount
}

// TCP接続テストモード（macOS非root権限用）
func runTcpConnectionTest(destinationHost string, destinationPort int, maxExecutionCount int) {
	var rtts []time.Duration
	successCount := 0
	
	fmt.Printf("TCP接続テスト: %s:%d\n", destinationHost, destinationPort)
	
	for i := 0; i < maxExecutionCount; i++ {
		start := time.Now()
		
		// TCP接続をテスト
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", destinationHost, destinationPort), 3*time.Second)
		rtt := time.Since(start)
		
		if err != nil {
			fmt.Printf("接続テスト %d: 失敗 - %v\n", i+1, err)
		} else {
			conn.Close()
			rtts = append(rtts, rtt)
			successCount++
			fmt.Printf("接続テスト %d: 成功 - rtt=%.2f ms\n", i+1, float64(rtt.Microseconds())/1000)
		}
		
		if i < maxExecutionCount-1 {
			time.Sleep(1 * time.Second)
		}
	}
	
	// 結果表示
	fmt.Printf("\n--- %s TCP接続テスト結果 ---\n", destinationHost)
	fmt.Printf("%d 接続試行, %d 成功, %.2f%% 失敗率\n",
		maxExecutionCount, successCount,
		float64(maxExecutionCount-successCount)/float64(maxExecutionCount)*100)
		
	if len(rtts) > 0 {
		minRtt, maxRtt, sumRtt := rtts[0], rtts[0], time.Duration(0)
		for _, rtt := range rtts {
			if rtt < minRtt {
				minRtt = rtt
			}
			if rtt > maxRtt {
				maxRtt = rtt
			}
			sumRtt += rtt
		}
		fmt.Printf("接続時間 min/avg/max = %.2f/%.2f/%.2f ms\n",
			float64(minRtt.Microseconds())/1000,
			float64(sumRtt.Microseconds())/float64(len(rtts))/1000,
			float64(maxRtt.Microseconds())/1000)
	}
}
