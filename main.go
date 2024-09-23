package main

import (
    "flag"
    "fmt"
    "net"
    "os"
    "time"
)

func main() {
    // 引数の設定
    count := flag.Int("c", 5, "Number of packets to send")
    port := flag.Int("p", 80, "Target port")
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

    // オプションの表示
    fmt.Printf("HPING %s (%s): %d packets\n", host, host, *count)
    fmt.Printf("Flags - SYN: %t, ACK: %t, PUSH: %t\n", *synFlag, *ackFlag, *pushFlag)

    // 送信、受信、RTTの初期化
    var rttTimes []time.Duration
    packetsSent := 0
    packetsReceived := 0

    // パケット送信ループ
    for i := 0; i < *count; i++ {
        start := time.Now()

        conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, *port))
        if err != nil {
            fmt.Printf("Request timeout for icmp_seq %d\n", i+1)
            continue
        }
        defer conn.Close()

        // パケット送信
        packetsSent++
        rtt := time.Since(start)
        rttTimes = append(rttTimes, rtt)
        packetsReceived++

        // 送信したパケットのRTTを表示
        fmt.Printf("64 bytes from %s: icmp_seq=%d ttl=64 time=%.2f ms\n", host, i+1, float64(rtt.Milliseconds()))

        // TCPフラグの設定（SYN, ACK, PUSH）
        if *synFlag {
            fmt.Println("[SYN flag set]")
            // SYNフラグ送信ロジックをここに追加
        }
        if *ackFlag {
            fmt.Println("[ACK flag set]")
            // ACKフラグ送信ロジックをここに追加
        }
        if *pushFlag {
            fmt.Println("[PUSH flag set]")
            // PUSHフラグ送信ロジックをここに追加
        }

        time.Sleep(1 * time.Second) // 1秒待機
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
