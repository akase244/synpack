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

    // ソケットを作成し、使い回し可能にする
    fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
    if err != nil {
        fmt.Printf("Error creating socket: %v\n", err)
        return
    }
    defer syscall.Close(fd)

    for i := 0; i < *count; i++ {
        packetsSent++
        seqNum := rand.Intn(10000) // シーケンス番号のランダム生成

        start := time.Now()
        err := sendTCPPacket(fd, ipAddr.IP, *port, *srcPort, seqNum, *synFlag, *ackFlag, *pushFlag)
        if err != nil {
            fmt.Printf("Request timeout for icmp_seq %d\n", i+1)
            continue
        }

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

// TCPパケットを送信する
func sendTCPPacket(fd int, dstIP net.IP, dstPort, srcPort, seqNum int, syn, ack, push bool) error {
    // 送信先アドレスの設定
    addr := syscall.SockaddrInet4{
        Port: dstPort,
    }
    copy(addr.Addr[:], dstIP.To4())

    // パケットの組み立て (シーケンス番号とフラグを設定)
    packet := createTCPPacket(srcPort, dstPort, seqNum, syn, ack, push)

    // パケットの送信
    err := syscall.Sendto(fd, packet, 0, &addr)
    if err != nil {
        return fmt.Errorf("failed to send packet: %v", err)
    }

    return nil
}

// TCPパケットの生成
func createTCPPacket(srcPort, dstPort, seqNum int, syn, ack, push bool) []byte {
    packet := make([]byte, 20) // TCPヘッダーサイズ

    // ソースポート
    packet[0] = byte(srcPort >> 8)
    packet[1] = byte(srcPort)

    // デスティネーションポート
    packet[2] = byte(dstPort >> 8)
    packet[3] = byte(dstPort)

    // シーケンス番号
    packet[4] = byte(seqNum >> 24)
    packet[5] = byte(seqNum >> 16)
    packet[6] = byte(seqNum >> 8)
    packet[7] = byte(seqNum)

    // 確認応答番号（通常は0）
    packet[8] = 0
    packet[9] = 0
    packet[10] = 0
    packet[11] = 0

    // データオフセット + フラグ + ウィンドウサイズ
    packet[12] = 5 << 4 // データオフセット
    flags := 0
    if syn {
        flags |= 0x02 // SYNフラグ
    }
    if ack {
        flags |= 0x10 // ACKフラグ
    }
    if push {
        flags |= 0x08 // PUSHフラグ
    }
    packet[13] = byte(flags) // TCPフラグ

    // ウィンドウサイズ
    packet[14] = 0
    packet[15] = 0

    // チェックサム（ここでは0を仮設定）
    packet[16] = 0
    packet[17] = 0

    // 緊急ポインタ（仮に0）
    packet[18] = 0
    packet[19] = 0

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
