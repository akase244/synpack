package main

import (
	"encoding/binary"
	"fmt"
	"flag"
	"net"
	"os"
	"syscall"
	"strings"
	"time"
	"math/rand"
)

func main() {
	// 引数を取得
	argHost := flag.String("host", "www.yahoo.co.jp", "送信先のホスト名")
	argPort := flag.Int("port", 443, "送信先のポート番号")
	flag.Parse()

	// 送信先ホスト名
	destinationHost := *argHost
	// 送信先ポート番号
	destinationPort := *argPort

	// ローカルIPアドレス
	localIpAddress := getLocalIpAddress()
	if localIpAddress == "" {
		fmt.Println("ローカルで有効なIPアドレスを取得できません")
		os.Exit(1)
	}
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
	// 送信元ポート番号
	sourcePort := generateAvailablePort()
	if sourcePort == 0 {
		fmt.Println("ローカルで有効なポート番号を取得できません")
		os.Exit(1)
	}

	// シーケンス番号をランダムに取得
	rand.Seed(time.Now().UnixNano())
	seqNumber := rand.Uint32()

	fmt.Println("送信元IPアドレス:", sourceIpAddress)
	fmt.Println("送信元ポート番号:", sourcePort)
	fmt.Println("送信先IPアドレス:", destinationIpAddress)
	fmt.Println("送信先ポート番号:", destinationPort)
	fmt.Println("シーケンス番号:", seqNumber)

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
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
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

	// SYNパケットを送信
	if err := syscall.Sendto(fd, packet, 0, &addr); err != nil {
		fmt.Println("syscall.Sendto実行時にエラーが発生しました", err)
		os.Exit(1)
	}
	fmt.Println("SYNパケットを送信しました")

	// SYN-ACKパケットを受信
	buf := make([]byte, 4096)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			fmt.Println("syscall.Recvfrom実行時にエラーが発生しました", err)
			os.Exit(1)
		}

		// 受信パケットを解析
		if err := parseAndVerifyPacket(buf[:n], sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber); err == nil {
			// SYN-ACK確認成功
			os.Exit(0)
		}
	}
}

// SYNパケットを生成
func createSynPacket(sourceIpAddress, destinationIpAddress net.IP, sourcePort, destinationPort int, seqNumber uint32) []byte {
	ipHeader := createIpHeader(sourceIpAddress, destinationIpAddress)
	tcpHeader := createTcpHeader(sourceIpAddress, destinationIpAddress, sourcePort, destinationPort, seqNumber)
	return append(ipHeader, tcpHeader...)
}

// IPヘッダーを生成
func createIpHeader(sourceIpAddress, destinationIpAddress net.IP) []byte {
	header := make([]byte, 20)
	header[0] = 0x45 // バージョン(4) + ヘッダー長(5)
	header[1] = 0x00 // サービスタイプ
	header[2], header[3] = 0x00, 0x28 // 全長 (40バイト)
	header[4], header[5] = 0x00, 0x00 // 識別子
	header[6], header[7] = 0x40, 0x00 // フラグメントオフセット
	header[8] = 0x40 // TTL
	header[9] = syscall.IPPROTO_TCP // プロトコル
	copy(header[12:16], sourceIpAddress.To4())
	copy(header[16:20], destinationIpAddress.To4())

	// チェックサム計算
	checksum := calcChecksum(header)
	header[10], header[11] = byte(checksum>>8), byte(checksum&0xff)
	return header
}

// TCPヘッダーを生成
func createTcpHeader(sourceIpAddress, destinationIpAddress net.IP, sourcePort, destinationPort int, seqNumber uint32) []byte {
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], uint16(sourcePort))
	binary.BigEndian.PutUint16(header[2:4], uint16(destinationPort))
	binary.BigEndian.PutUint32(header[4:8], seqNumber)
	header[12] = 0x50 // ヘッダー長(5)
	header[13] = 0x02 // SYNフラグ
	header[14], header[15] = 0x72, 0x10 // ウィンドウサイズ

	// チェックサム計算
	pseudoHeader := createPseudoHeader(sourceIpAddress, destinationIpAddress, header)
	checksum := calcChecksum(pseudoHeader)
	header[16], header[17] = byte(checksum>>8), byte(checksum&0xff)
	return header
}

// 疑似ヘッダーを生成
func createPseudoHeader(sourceIpAddress, destinationIpAddress net.IP, tcpHeader []byte) []byte {
	pseudoHeader := make([]byte, 12+len(tcpHeader))
	copy(pseudoHeader[0:4], sourceIpAddress.To4())
	copy(pseudoHeader[4:8], destinationIpAddress.To4())
	pseudoHeader[8] = 0x00 // 予約
	pseudoHeader[9] = syscall.IPPROTO_TCP
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(len(tcpHeader)))
	copy(pseudoHeader[12:], tcpHeader)
	return pseudoHeader
}

// パケットを解析してSYN-ACKを確認
func parseAndVerifyPacket(buf []byte, sourceIpAddress, destinationIpAddress net.IP, sourcePort, destinationPort int, seqNumberSent uint32) error {
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
		fmt.Printf("SYN-ACKを受信しました: %s:%d -> %s:%d\n", sourceIp, sourcePortRecv, destinationIp, destinationPortRecv)
		fmt.Println("ACK番号:", ackNumberRecv)
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

// ローカル環境のネットワークインターフェースからIPアドレスを取得
func getLocalIpAddress() string {
	// インターフェースを取得
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		// 以下のインターフェースの場合はスキップ
		// - 無効状態
		// - ループバックアドレス
		// - インターフェース名がDocker関連
		if (iface.Flags & net.FlagUp) == 0 || (iface.Flags & net.FlagLoopback) != 0 || isDockerInterface(iface.Name) {
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
			return ip.String()
		}
	}
	return ""
}

// Docker関連のインターフェース名か判定する
func isDockerInterface(name string) bool {
	// Dockerで使われる典型的なインターフェース名
	dockerPrefixes := []string{"docker", "br-", "veth", "tunl", "flannel"}

	for _, prefix := range dockerPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ローカルで利用可能なポート番号を取得
func generateAvailablePort() int {
	retryCount := 10
	for i := 0; i < retryCount; i++ {
		// ポート番号をランダムに取得
		rand.Seed(time.Now().UnixNano())
		port := rand.Intn(65535-49152+1) + 49152
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