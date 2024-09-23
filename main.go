package main

import (
    "flag"
    "fmt"
    "math/rand"
    "net"
    "os"
    "syscall"
    "time"
)

func main() {
    // コマンドライン引数の設定
    count := flag.Int("c", 5, "Number of packets to send")
    port := flag.Int("p", 80, "Target port")
    srcPort := flag.Int("s", 0, "Source port (optional)")
    synFlag := flag.Bool("S", false, "Set SYN flag")
    ackFlag := flag.Bool("A", false, "Set ACK flag")
    pushFlag := flag.Bool("P", false, "Set PUSH flag")

    // ホスト名は最後の引数で指定
    flag.Parse()
    if len(flag.Args()) < 1 {
        fmt.Println("Error: Host must be specified")
        os.Exit(1)
    }
    host := flag.Args()[0]

    // 送信元ポートが指定されなければランダムなポートを使う
    if *srcPort == 0 {
        *srcPort = rand.Intn(65535-1024) + 1024 // 1024から65535の間でランダムに選択
    }

    // オプションの表示
    fmt.Printf("HPING %s (%s): %d packets from port %d\n", host, host, *count, *srcPort)
    fmt.Printf("Flags - SYN: %t, ACK: %t, PUSH: %t\n", *synFlag, *ackFlag, *pushFlag)

    var rttTimes []time.Duration
    packetsSent := 0
    packetsReceived := 0

    // IPアドレスの解決
    ipAddr, err := net.ResolveIPAddr("ip", host)
    if err != nil {
        fmt.Printf("Could not resolve host: %v\n", err)
        return
    }

    for i := 0; i < *count; i++ {
        packetsSent++
        seqNum := rand.Intn(10000) // シーケンス番号のランダム生成

        start := time.Now()
        conn, err := sendTCPPacket(ipAddr.IP, *port, *srcPort, seqNum, *synFlag, *ackFlag, *pushFlag)
        if err != nil {
            fmt.Printf("Request timeout for icmp_seq %d\n", i+1)
            continue
        }
        defer conn.Close()

        rtt := time.Since(start)
        rttTimes = append(rttTimes, rtt)
        packetsReceived++

        fmt.Printf("64 bytes from %s: icmp_seq=%d ttl=64 time=%.2f ms\n", host, i+1, float64(rtt.Milliseconds()))

        time.Sleep(1 * time.Second)
    }

    // 統計情報の表示
    fmt.Println("\n---", host, "ping statistics ---")
    fmt.Printf("%d packets transmitted, %d received, %.1f%% packet loss\n",
        packetsSent, packetsReceived, float64(packetsSent-packetsReceived)/float64(packetsSent)*100)

    if len(rttTimes) > 0 {
        minRTT, maxRTT, avgRTT := calculateRTTStats(rttTimes)
        fmt.Printf("rtt min/avg/max = %.2f/%.2f/%.2f ms\n", float64(minRTT.Milliseconds()), float64(avgRTT.Milliseconds()), float64(maxRTT.Milliseconds()))
    }
}

// TCPパケットを送信し、ソケット接続を返す
func sendTCPPacket(dstIP net.IP, dstPort, srcPort, seqNum int, syn, ack, push bool) (net.Conn, error) {
    // ソケットの作成
    fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
    if err != nil {
        return nil, fmt.Errorf("failed to create raw socket: %v", err)
    }
    defer syscall.Close(fd)

    // 送信先アドレスの設定
    addr := syscall.SockaddrInet4{
        Port: dstPort,
    }
    copy(addr.Addr[:], dstIP.To4())

    // パケットの組み立て (ここでシーケンス番号とフラグを指定)
    packet := createTCPPacket(srcPort, dstPort, seqNum, syn, ack, push)

    // パケットの送信
    err = syscall.Sendto(fd, packet, 0, &addr)
    if err != nil {
        return nil, fmt.Errorf("failed to send packet: %v", err)
    }

    // TCP接続の確認（シーケンス番号や応答を後で追加処理）
    conn, err := net.DialTCP("tcp", &net.TCPAddr{Port: srcPort}, &net.TCPAddr{IP: dstIP, Port: dstPort})
    if err != nil {
        return nil, fmt.Errorf("failed to establish connection: %v", err)
    }

    return conn, nil
}

// TCPパケットの生成
func createTCPPacket(srcPort, dstPort, seqNum int, syn, ack, push bool) []byte {
    // TCPヘッダやフラグを作成 (実際のフラグ設定処理はここで行う)
    // 必要に応じて、TCPヘッダを低レベルで構築する

    // ここでは仮にパケットのバイトスライスを返している
    packet := make([]byte, 20) // TCPヘッダ20バイトを生成
    return packet
}

// RTT統計情報の計算
func calculateRTTStats(times []time.Duration) (min, max, avg time.Duration) {
    min, max = times[0], times[0]
    var sum time.Duration

    for _, t := range times {
        if t < min {
            min = t
        }
        if t > max {
            max = t
        }
        sum += t
    }
    avg = sum / time.Duration(len(times))
    return
}
