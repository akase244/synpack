package main

import (
    "flag"
    "fmt"
    "net"
    "time"
)

func main() {
    // 引数としてホスト、ポート、回数、TCPフラグを指定
    host := flag.String("host", "example.com", "Target host")
    port := flag.String("port", "80", "Target port")
    count := flag.Int("c", 5, "Number of packets to send")
    synFlag := flag.Bool("syn", false, "Send SYN flag")
    ackFlag := flag.Bool("ack", false, "Send ACK flag")
    pushFlag := flag.Bool("push", false, "Send PUSH flag")
    flag.Parse()

    // TCPフラグの設定を確認
    fmt.Printf("Host: %s, Port: %s, Count: %d\n", *host, *port, *count)
    fmt.Printf("Flags - SYN: %t, ACK: %t, PUSH: %t\n", *synFlag, *ackFlag, *pushFlag)

    var rttTimes []time.Duration

    for i := 0; i < *count; i++ {
        start := time.Now()

        conn, err := net.Dial("tcp", *host+":"+*port)
        if err != nil {
            fmt.Println("Error:", err)
            continue
        }
        defer conn.Close()

        // RTTを計測
        rtt := time.Since(start)
        fmt.Printf("RTT: %v\n", rtt)
        rttTimes = append(rttTimes, rtt)

        // Optional: TCPフラグを設定
        if *synFlag {
            fmt.Println("Sending SYN flag")
            // SYNフラグ送信処理を追加
        }
        if *ackFlag {
            fmt.Println("Sending ACK flag")
            // ACKフラグ送信処理を追加
        }
        if *pushFlag {
            fmt.Println("Sending PUSH flag")
            // PUSHフラグ送信処理を追加
        }

        // 必要であれば応答の確認や詳細な処理を追加
    }

    // RTTの統計情報を計算
    if len(rttTimes) > 0 {
        minRTT, maxRTT, avgRTT := calculateRTTStats(rttTimes)
        fmt.Printf("Min RTT: %v, Max RTT: %v, Avg RTT: %v\n", minRTT, maxRTT, avgRTT)
    }
}

// RTTの統計情報を計算する関数
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
