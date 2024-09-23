package main

import (
    "flag"
    "fmt"
    "net"
    "os"
    "time"
)

func main() {
    // コマンドライン引数を設定 (nping, hping3風)
    count := flag.Int("c", 5, "Number of packets to send")
    port := flag.Int("p", 80, "Target port")
    synFlag := flag.Bool("S", false, "Set SYN flag")
    ackFlag := flag.Bool("A", false, "Set ACK flag")
    pushFlag := flag.Bool("P", false, "Set PUSH flag")

    // ホスト名は最後の引数として取得
    flag.Parse()
    if len(flag.Args()) < 1 {
        fmt.Println("Error: Host must be specified")
        os.Exit(1)
    }
    host := flag.Args()[0]

    // オプションの確認
    fmt.Printf("Host: %s, Port: %d, Count: %d\n", host, *port, *count)
    fmt.Printf("Flags - SYN: %t, ACK: %t, PUSH: %t\n", *synFlag, *ackFlag, *pushFlag)

    // RTT計測
    var rttTimes []time.Duration
    for i := 0; i < *count; i++ {
        start := time.Now()

        conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, *port))
        if err != nil {
            fmt.Println("Error:", err)
            continue
        }
        defer conn.Close()

        // RTTの計測
        rtt := time.Since(start)
        fmt.Printf("RTT: %v\n", rtt)
        rttTimes = append(rttTimes, rtt)

        // TCPフラグの設定 (Raw socket等で拡張可能)
        if *synFlag {
            fmt.Println("SYN flag is set")
            // SYNフラグ送信ロジックを追加
        }
        if *ackFlag {
            fmt.Println("ACK flag is set")
            // ACKフラグ送信ロジックを追加
        }
        if *pushFlag {
            fmt.Println("PUSH flag is set")
            // PUSHフラグ送信ロジックを追加
        }
    }

    // RTTの統計情報を計算
    if len(rttTimes) > 0 {
        minRTT, maxRTT, avgRTT := calculateRTTStats(rttTimes)
        fmt.Printf("Min RTT: %v, Max RTT: %v, Avg RTT: %v\n", minRTT, maxRTT, avgRTT)
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
