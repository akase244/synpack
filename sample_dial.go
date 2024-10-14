package main

import (
	"flag"
	"fmt"
	"net"
	"time"
)

func main() {
	host := flag.String("host", "example.com", "Target host")
	port := flag.String("port", "80", "Target port")
	count := flag.Int("c", 5, "Number of packets to send")
	flag.Parse()

	var rttTimes []time.Duration

	for i := 0; i < *count; i++ {
		start := time.Now()

		conn, err := net.Dial("tcp", *host+":"+*port)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		defer conn.Close()

		rtt := time.Since(start)
		fmt.Printf("RTT: %v\n", rtt)
		rttTimes = append(rttTimes, rtt)

		// Optional: Add logic to send custom TCP flags (e.g., SYN, ACK, etc.)
		// using raw sockets or libraries like golang.org/x/net/ipv4 for low-level control
	}

	// Calculate min, max, avg RTT
	if len(rttTimes) > 0 {
		minRTT, maxRTT, avgRTT := calculateRTTStats(rttTimes)
		fmt.Printf("Min RTT: %v, Max RTT: %v, Avg RTT: %v\n", minRTT, maxRTT, avgRTT)
	}
}

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
