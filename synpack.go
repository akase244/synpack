package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 引数を取得
	argHost := flag.String("h", "", "送信先のホスト名(必須)")
	argPort := flag.Int("p", 0, "送信先のポート番号(必須:0-65535)")
	argCount := flag.Int("c", 0, "実行回数")
	flag.Parse()

	// シグナル受信設定
	signalChan := make(chan os.Signal, 1)
	// [ctrl+c]をキャッチする
	signal.Notify(signalChan, os.Interrupt)

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
	if *argCount < 0 {
		fmt.Fprintln(os.Stderr, "実行回数は1以上を指定してください")
		os.Exit(1)
	}

	// 送信先ホスト名
	destinationHost := *argHost
	// 送信先ポート番号
	destinationPort := *argPort
	// 実行回数
	retryCount := *argCount

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

	fmt.Printf("Synpack %s (%s) -> %s (%s)\n", sourceInterfaceName, sourceIpAddress, destinationHost, destinationIpAddress)
	executionCount := 0
	receivedCount := 0
	for {
		if retryCount > 0 && executionCount >= retryCount {
			break
		}
		if shouldExit {
			break
		}

		// 実行回数をインクリメント
		executionCount++

		// 送信元ポート番号（毎実行時にポート番号を変える）
		sourcePort := generateAvailablePort()
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

		// ソケットを作成
		// - アドレスファミリー:IPv4
		// - ソケットの種類:低レベルソケット
		// - プロトコル:TCP
		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
		if err != nil {
			fmt.Println("syscall.Socket実行時にエラーが発生しました", err)
			os.Exit(1)
		}
		defer syscall.Close(fd)

		// ソケットオプションでIPヘッダーを手動生成に設定
		err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
		if err != nil {
			fmt.Println("syscall.SetsockoptInt実行時にエラーが発生しました", err)
			os.Exit(1)
		}

		// SYNパケットを生成
		packet := createSynPacket(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber)

		// 宛先アドレス設定
		addr := syscall.SockaddrInet4{
			Port: destinationPort,
		}
		copy(addr.Addr[:], destinationIpAddress.To4())

		start := time.Now()
		// SYNパケットを送信
		err = syscall.Sendto(fd, packet, 0, &addr)
		if err != nil {
			fmt.Println("syscall.Sendto実行時にエラーが発生しました", err)
			os.Exit(1)
		}

		// SYN-ACKパケットを受信
		buf := make([]byte, 4096)

		// タイムアウトの設定
		timeout := 3 * time.Second

		// チャネルを使ってタイムアウトと受信を監視
		timeoutChannel := time.After(timeout)
		receiveChannel := make(chan []byte)

		go func() {
			for {
				n, _, err := syscall.Recvfrom(fd, buf, 0)
				if err != nil {
					fmt.Println("syscall.Recvfrom実行時にエラーが発生しました", err)
					os.Exit(1)
				}

				// データ受信時にチャネルに送る
				receiveChannel <- buf[:n]
			}
		}()

		select {
		case receivedPacket := <-receiveChannel:
			// 受信パケットを解析
			err = parseAndVerifyPacket(receivedPacket, sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber)
			if err == nil {
				// SYN-ACK確認成功
				receivedCount++
				rtt := time.Since(start)
				rtts = append(rtts, rtt)
				fmt.Printf("len=%d ip=%s port=%d seq=%d rtt=%.2fms\n", len(buf), destinationIpAddress, destinationPort, seqNumber, float64(rtt.Microseconds())/1000)

				// ACK番号を取得
				tcpHeader := receivedPacket[20:40]
				ackNumber := binary.BigEndian.Uint32(tcpHeader[8:12])

				// RSTパケットを生成
				rstPacket := createRstPacket(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber, ackNumber)

				// RSTパケットを送信
				err = syscall.Sendto(fd, rstPacket, 0, &addr)
				if err != nil {
					fmt.Println("syscall.Sendto実行時にエラーが発生しました", err)
					os.Exit(1)
				}
			}
		case <-timeoutChannel:
			// 何もしない
		}

		time.Sleep(1 * time.Second)
	}

	// RTT結果を表示
	fmt.Printf("\n--- %s Synpack statistic ---\n", destinationHost)
	fmt.Printf("%d packets transmitted, %d packets received, %.2f%% packet loss\n",
		retryCount, receivedCount,
		(float64(retryCount-receivedCount)/float64(retryCount))*100)
	if len(rtts) > 0 {
		minRTT, maxRTT, sumRTT := rtts[0], rtts[0], time.Duration(0)
		for _, rtt := range rtts {
			if rtt < minRTT {
				minRTT = rtt
			}
			if rtt > maxRTT {
				maxRTT = rtt
			}
			sumRTT += rtt
		}
		fmt.Printf("round-trip min/avg/max = %.2fms/%.2fms/%.2fms\n",
			float64(minRTT.Microseconds())/1000,
			float64(sumRTT.Microseconds())/float64(len(rtts))/1000,
			float64(maxRTT.Microseconds())/1000)
	} else {
		fmt.Printf("round-trip min/avg/max = 0.0/0.0/0.0\n")
	}
}

// SYNパケットを生成
func createSynPacket(sourceIpAddress net.IP, destinationIpAddress net.IP, sourcePort int, destinationPort int, seqNumber uint32) []byte {
	ipHeader := createIpHeader(sourceIpAddress, destinationIpAddress)
	tcpHeader := createSynTcpHeader(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber)
	return append(ipHeader, tcpHeader...)
}

// IPヘッダーを生成
func createIpHeader(sourceIpAddress net.IP, destinationIpAddress net.IP) []byte {
	header := make([]byte, 20)
	header[0] = 0x45                  // バージョン(4) + ヘッダー長(5)
	header[1] = 0x00                  // サービスタイプ
	header[2], header[3] = 0x00, 0x28 // 全長 (40バイト)
	header[4], header[5] = 0x00, 0x00 // 識別子
	header[6], header[7] = 0x40, 0x00 // フラグメントオフセット
	header[8] = 0x40                  // TTL
	header[9] = syscall.IPPROTO_TCP   // プロトコル
	copy(header[12:16], sourceIpAddress.To4())
	copy(header[16:20], destinationIpAddress.To4())

	// チェックサム計算
	checksum := calcChecksum(header)
	header[10], header[11] = byte(checksum>>8), byte(checksum&0xff)
	return header
}

// TCPヘッダーを生成
func createSynTcpHeader(sourceIpAddress net.IP, destinationIpAddress net.IP, sourcePort int, destinationPort int, seqNumber uint32) []byte {
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], uint16(sourcePort))
	binary.BigEndian.PutUint16(header[2:4], uint16(destinationPort))
	binary.BigEndian.PutUint32(header[4:8], seqNumber)
	header[12] = 0x50                   // ヘッダー長(5)
	header[13] = 0x02                   // SYNフラグ
	header[14], header[15] = 0x72, 0x10 // ウィンドウサイズ

	// チェックサム計算
	pseudoHeader := createPseudoHeader(sourceIpAddress, destinationIpAddress, header)
	checksum := calcChecksum(pseudoHeader)
	header[16], header[17] = byte(checksum>>8), byte(checksum&0xff)
	return header
}

// 疑似ヘッダーを生成
func createPseudoHeader(sourceIpAddress net.IP, destinationIpAddress net.IP, tcpHeader []byte) []byte {
	pseudoHeader := make([]byte, 12+len(tcpHeader))
	copy(pseudoHeader[0:4], sourceIpAddress.To4())
	copy(pseudoHeader[4:8], destinationIpAddress.To4())
	pseudoHeader[8] = 0x00 // 予約
	pseudoHeader[9] = syscall.IPPROTO_TCP
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(len(tcpHeader)))
	copy(pseudoHeader[12:], tcpHeader)
	return pseudoHeader
}

// RSTパケットを生成
func createRstPacket(sourceIpAddress net.IP, destinationIpAddress net.IP, sourcePort int, destinationPort int, seqNumber uint32, ackNumber uint32) []byte {
	ipHeader := createIpHeader(sourceIpAddress, destinationIpAddress)
	tcpHeader := createRstTcpHeader(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber, ackNumber)
	return append(ipHeader, tcpHeader...)
}

// TCPヘッダーを生成（RSTフラグ付き）
func createRstTcpHeader(sourceIpAddress net.IP, destinationIpAddress net.IP, sourcePort int, destinationPort int, seqNumber uint32, ackNumber uint32) []byte {
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], uint16(sourcePort))      // 送信元ポート
	binary.BigEndian.PutUint16(header[2:4], uint16(destinationPort)) // 宛先ポート
	binary.BigEndian.PutUint32(header[4:8], seqNumber)               // シーケンス番号
	binary.BigEndian.PutUint32(header[8:12], ackNumber)              // ACK番号
	header[12] = 0x50                                                // ヘッダー長(5)
	header[13] = 0x04                                                // RSTフラグ
	header[14], header[15] = 0x00, 0x00                              // ウィンドウサイズ

	// チェックサム計算
	pseudoHeader := createPseudoHeader(sourceIpAddress, destinationIpAddress, header)
	checksum := calcChecksum(pseudoHeader)
	header[16], header[17] = byte(checksum>>8), byte(checksum&0xff)
	return header
}

// パケットを解析してSYN-ACKを確認
func parseAndVerifyPacket(buf []byte, sourceIpAddress net.IP, destinationIpAddress net.IP, sourcePort int, destinationPort int, seqNumberSent uint32) error {
	if len(buf) < 40 {
		return fmt.Errorf("パケットが短すぎます")
	}

	// IPヘッダー解析
	sourceIp := net.IP(buf[12:16])
	destinationIp := net.IP(buf[16:20])
	if !sourceIp.Equal(destinationIpAddress) || !destinationIp.Equal(sourceIpAddress) {
		return fmt.Errorf("IPアドレスが一致しません")
	}

	// TCPヘッダー解析
	tcpHeader := buf[20:40]
	sourcePortRecv := int(binary.BigEndian.Uint16(tcpHeader[0:2]))
	destinationPortRecv := int(binary.BigEndian.Uint16(tcpHeader[2:4]))
	flags := tcpHeader[13]

	// シーケンス番号
	//seqNumberRecv := binary.BigEndian.Uint32(tcpHeader[4:8])  // 4バイト目から
	// ACK番号
	ackNumberRecv := binary.BigEndian.Uint32(tcpHeader[8:12]) // 8バイト目から

	if ackNumberRecv == seqNumberSent+1 && sourcePortRecv == destinationPort && destinationPortRecv == sourcePort && (flags&0x12) == 0x12 {
		//fmt.Printf("SYN-ACKを受信しました: %s:%d -> %s:%d\n", sourceIp, sourcePortRecv, destinationIp, destinationPortRecv)
		//fmt.Println("ACK番号:", ackNumberRecv)
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
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 || hasDockerInterfaceName(iface.Name) {
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

// ローカルで利用可能なポート番号を取得
func generateAvailablePort() int {
	// 再実行の回数
	retryCount := 5
	// ポート番号の範囲
	minPort := 49152
	maxPort := 65535
	for i := 0; i < retryCount; i++ {
		// ポート番号をランダムに取得
		n, err := rand.Int(rand.Reader, big.NewInt(int64(maxPort-minPort+1)))
		if err != nil {
			return 0
		}
		port := int(n.Int64()) + minPort
		// ポートが空いているか確認
		if isAvailablePort(port) {
			return port
		}
	}
	return 0
}

func isAvailablePort(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	// LISTEN不可の場合は利用できない（他の処理で利用中のポート）
	if err != nil {
		return false
	}
	defer listener.Close()
	return true
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
