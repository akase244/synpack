package main

import (
	"encoding/binary"
	"net"
	"reflect"
	"syscall"
	"testing"
)

func TestHasDockerInterfaceName(t *testing.T) {
	tests := []struct {
		input  string
		expect bool
	}{
		{"docker0", true},
		{"br-123abc456def", true},
		{"veth0abc1", true},
		{"tunl0", true},
		{"flannel.1", true},
		{"cni0", true},
		{"eth0", false},
		{"wlp2s0", false},
	}

	for _, test := range tests {
		acutual := hasDockerInterfaceName(test.input)
		if acutual != test.expect {
			t.Errorf("hasDockerInterfaceName(%q) = %v; expect %v", test.input, acutual, test.expect)
		}
	}
}

func TestCreateIpHeader(t *testing.T) {
	sourceIp := net.IPv4(192, 168, 0, 1)
	destinationIp := net.IPv4(8, 8, 8, 8)

	expectedHeader := []byte{
		0x45,       // バージョン + ヘッダー長
		0x00,       // サービスタイプ
		0x00, 0x28, // 全長
		0x00, 0x00, // 識別子
		0x40, 0x00, // フラグメントオフセット
		0x40,                // TTL
		syscall.IPPROTO_TCP, // プロトコル
		0x00, 0x00,          // チェックサム (後で計算される)
		192, 168, 0, 1, // 送信元IPアドレス
		8, 8, 8, 8, // 送信先IPアドレス
	}

	// 実際に作成したヘッダー
	actualHeader := createIpHeader(sourceIp, destinationIp)

	// チェックサムの部分をテストするために計算
	expectedChecksum := calcChecksum(expectedHeader)
	expectedHeader[10] = byte(expectedChecksum >> 8)
	expectedHeader[11] = byte(expectedChecksum & 0xff)

	// 検証
	if !reflect.DeepEqual(actualHeader, expectedHeader) {
		t.Errorf("createIpHeader(%q, %q) = %v; expect %v", sourceIp, destinationIp, actualHeader, expectedHeader)
	}
}

func TestCreateTcpHeader(t *testing.T) {
	sourceIp := net.IPv4(192, 168, 0, 1)
	destinationIp := net.IPv4(8, 8, 8, 8)
	sourcePort := 54321
	destinationPort := 80
	seqNumber := uint32(12345)

	expectedHeader := make([]byte, 20)
	binary.BigEndian.PutUint16(expectedHeader[0:2], uint16(sourcePort))      // 送信元ポート
	binary.BigEndian.PutUint16(expectedHeader[2:4], uint16(destinationPort)) // 送信先ポート
	binary.BigEndian.PutUint32(expectedHeader[4:8], seqNumber)               // シーケンス番号
	expectedHeader[12] = 0x50                                                // ヘッダー長
	expectedHeader[13] = 0x02                                                // SYNフラグ
	expectedHeader[14], expectedHeader[15] = 0x72, 0x10                      // ウィンドウサイズ

	// 実際のヘッダー作成
	actualHeader := createTcpHeader(sourceIp, destinationIp, sourcePort, destinationPort, seqNumber)

	// チェックサム計算
	pseudoHeader := createPseudoHeader(sourceIp, destinationIp, expectedHeader)
	checksum := calcChecksum(pseudoHeader)
	expectedHeader[16] = byte(checksum >> 8)   // チェックサム (上位バイト)
	expectedHeader[17] = byte(checksum & 0xff) // チェックサム (下位バイト)

	if !reflect.DeepEqual(actualHeader, expectedHeader) {
		t.Errorf("createTcpHeader(%q, %q, %d, %d, %d) = %v; expect %v", sourceIp, destinationIp, sourcePort, destinationPort, seqNumber, actualHeader, expectedHeader)
	}
}

func TestCreatePseudoHeader(t *testing.T) {
	sourceIp := net.ParseIP("192.168.0.1")
	destinationIp := net.ParseIP("8.8.8.8")
	tcpHeader := []byte{
		0xD4, 0x31, // 送信元ポート (54321)
		0x01, 0xbb, // 送信先ポート (443)
		0x00, 0x00, 0x30, 0x39, // シーケンス番号 (12345)
		0x00, 0x00, 0x00, 0x00, // ACK番号
		0x50,       // ヘッダー長
		0x02,       // SYNフラグ
		0x72, 0x10, // ウィンドウサイズ
		0x00, 0x00, // チェックサム（未設定）
		0x00, 0x00, // 緊急ポインタ
	}

	expected := make([]byte, 12+len(tcpHeader))
	copy(expected[0:4], sourceIp.To4())
	copy(expected[4:8], destinationIp.To4())
	expected[8] = 0x00                                                  // 予約
	expected[9] = syscall.IPPROTO_TCP                                   // プロトコル
	binary.BigEndian.PutUint16(expected[10:12], uint16(len(tcpHeader))) // TCPヘッダーの長さ
	copy(expected[12:], tcpHeader)

	actual := createPseudoHeader(sourceIp, destinationIp, tcpHeader)

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("createPseudoHeader(%q, %q, %v) = %v; expect %v", sourceIp, destinationIp, tcpHeader, actual, expected)
	}
}
