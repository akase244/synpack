package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

func main() {
	// インターフェース名の取得
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}

	var device string
	for _, dev := range devices {
		if dev.Name != "lo" && len(dev.Addresses) > 0 {
			device = dev.Name
			break
		}
	}

	// パケットキャプチャの設定
	handle, err := pcap.OpenLive(device, 65535, true, pcap.BlockForever)
	if err != nil {
		log.Fatal(err)
	}
	defer handle.Close()

	srcIP := net.ParseIP("192.168.0.132")
	dstIP := net.ParseIP("192.168.0.1")
	srcPort := layers.TCPPort(12345)
	dstPort := layers.TCPPort(80)

	//// フィルタの設定
	//filter := fmt.Sprintf("tcp and ((src host %s and src port %d) or (dst host %s and dst port %d))",
	//	dstIP.String(), dstPort, srcIP.String(), srcPort)
	//err = handle.SetBPFFilter(filter)
	//if err != nil {
	//	log.Fatal(err)
	//}

	// SYNパケットの作成と送信
	ip := &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Protocol: layers.IPProtocolTCP,
		Version:  4,
		TTL:      64,
		IHL:      5,
	}

	tcp := &layers.TCP{
		SrcPort: srcPort,
		DstPort: dstPort,
		Seq:     1000,
		SYN:     true,
		Window:  65535,
	}

	err = tcp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		log.Fatal(err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths: true,
	}

	err = gopacket.SerializeLayers(buf, opts, ip, tcp)
	if err != nil {
		log.Fatal(err)
	}

	err = handle.WritePacketData(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("SYN packet sent successfully")

	// パケットキャプチャの開始
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	timeout := time.After(5 * time.Second)

	for {
		select {
		case packet := <-packetSource.Packets():
			// デバッグ情報の出力
			fmt.Printf("Captured packet: %v\n", packet)

			ipLayer := packet.Layer(layers.LayerTypeIPv4)
			if ipLayer == nil {
				continue
			}
			ip, _ := ipLayer.(*layers.IPv4)

			tcpLayer := packet.Layer(layers.LayerTypeTCP)
			if tcpLayer == nil {
				continue
			}
			tcp, _ := tcpLayer.(*layers.TCP)

			fmt.Printf("IP: %s -> %s, Ports: %d -> %d, Flags: SYN=%v ACK=%v\n",
				ip.SrcIP, ip.DstIP, tcp.SrcPort, tcp.DstPort, tcp.SYN, tcp.ACK)

			if tcp.SYN && tcp.ACK {
				fmt.Println("Received SYN-ACK packet")
				return
			}

		case <-timeout:
			fmt.Println("Timeout waiting for SYN-ACK")
			return
		}
	}
}