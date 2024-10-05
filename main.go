package main

import (
    "flag"
    "fmt"
    "github.com/google/gopacket"
    "github.com/google/gopacket/layers"
    "github.com/google/gopacket/pcap"
    "net"
    "time"
)

func getLocalIP() (net.IP, error) {
    interfaces, err := net.Interfaces()
    if err != nil {
        return nil, err
    }

    for _, iface := range interfaces {
        addrs, err := iface.Addrs()
        if err != nil {
            continue
        }

        for _, addr := range addrs {
            if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
                if ipNet.IP.To4() != nil { // IPv4アドレスのみ
                    return ipNet.IP, nil
                }
            }
        }
    }
    return nil, fmt.Errorf("no valid network interface found")
}

func sendSynPacket(targetIP string, targetPort int) (time.Duration, error) {
    // 送信元のIPを取得
    srcIP, err := getLocalIP()
    if err != nil {
        return 0, err
    }

    // インターフェースの選択
    iface, err := net.InterfaceByName("en0") // 適切なインターフェースを指定
    if err != nil {
        return 0, err
    }

    // スニファーの設定
    handle, err := pcap.OpenLive(iface.Name, 1600, true, pcap.BlockForever)
    if err != nil {
        return 0, err
    }
    defer handle.Close()

    // TCP SYNパケットの作成
    ipLayer := &layers.IPv4{
        SrcIP:    srcIP,
        DstIP:    net.ParseIP(targetIP),
        Protocol: layers.IPProtocolTCP,
    }

    tcpLayer := &layers.TCP{
        SrcPort: layers.TCPPort(12345), // 自分のポートを指定
        DstPort: layers.TCPPort(targetPort),
        SYN:     true,
        Window:  65535,
    }

    // TCPヘッダーを計算
    tcpLayer.SetNetworkLayerForChecksum(ipLayer)

    buf := gopacket.NewSerializeBuffer()
    gopacket.SerializeLayers(buf, gopacket.SerializeOptions{}, ipLayer, tcpLayer)

    // パケットを送信
    start := time.Now()
    err = handle.WritePacketData(buf.Bytes())
    if err != nil {
        return 0, err
    }

    // 応答を待つ
    packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
    for packet := range packetSource.Packets() {
        // 応答をチェック
        if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
            tcp := tcpLayer.(*layers.TCP)
            if tcp.SYN && tcp.ACK {
                duration := time.Since(start)
                fmt.Printf("Received SYN-ACK from %s:%d in %v\n", targetIP, targetPort, duration)
                return duration, nil
            }
        }
    }

    return 0, fmt.Errorf("no SYN-ACK received from %s:%d", targetIP, targetPort)
}

func main() {
    //targetIP := "tsunagi.me" // 監視対象のIP
    //targetIP := "8.8.8.8" // 監視対象のIP
    targetIP := "www.google.com" // 監視対象のIP
    targetPort := 80         // 監視対象のポート
    count := flag.Int("c", 4, "number of packets to send") // 引数で実行回数を指定
    flag.Parse()

    var totalRTT time.Duration
    var minRTT time.Duration = time.Duration(1<<63 - 1) // 初期値として非常に大きな値を設定
    var maxRTT time.Duration

    for i := 0; i < *count; i++ {
        rtt, err := sendSynPacket(targetIP, targetPort)
        if err != nil {
            fmt.Println(err)
            continue
        }

        totalRTT += rtt
        if rtt < minRTT {
            minRTT = rtt
        }
        if rtt > maxRTT {
            maxRTT = rtt
        }

        time.Sleep(1 * time.Second) // 次のチェックまで1秒待つ
    }

    if totalRTT > 0 {
        avgRTT := totalRTT / time.Duration(*count)
        fmt.Printf("RTT: AVG: %v, MIN: %v, MAX: %v\n", avgRTT, minRTT, maxRTT)
    } else {
        fmt.Println("No RTT measurements were taken.")
    }
}

