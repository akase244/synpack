package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: go run main.go <host> <port> <count>")
		return
	}

	host := os.Args[1]
	port := os.Args[2]
	count, err := strconv.Atoi(os.Args[3])
	if err != nil || count <= 0 {
		fmt.Println("Invalid count:", os.Args[3])
		return
	}

	address := net.JoinHostPort(host, port)
	var rttTimes []time.Duration

	fmt.Printf("PING %s:%s (%s)\n", host, port, address)

	for i := 0; i < count; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		rtt := time.Since(start)

		if err != nil {
			fmt.Printf("Request %d: Connection failed: %s\n", i+1, err)
		} else {
			fmt.Printf("Request %d: Connected, RTT = %v\n", i+1, rtt)
			rttTimes = append(rttTimes, rtt)
			conn.Close()
		}

		// 1秒の間隔を空ける
		time.Sleep(1 * time.Second)
	}

	if len(rttTimes) > 0 {
		// RTTの統計計算
		min := rttTimes[0]
		max := rttTimes[0]
		var sum time.Duration

		for _, rtt := range rttTimes {
			if rtt < min {
				min = rtt
			}
			if rtt > max {
				max = rtt
			}
			sum += rtt
		}
		avg := sum / time.Duration(len(rttTimes))

		// 結果をソートして表示
		sort.Slice(rttTimes, func(i, j int) bool { return rttTimes[i] < rttTimes[j] })
		fmt.Println("\n---", address, "statistics ---")
		fmt.Printf("%d packets transmitted, %d packets received\n", count, len(rttTimes))
		fmt.Printf("RTT min/avg/max = %v/%v/%v\n", min, avg, max)
	} else {
		fmt.Println("\nNo successful connections.")
	}
}
