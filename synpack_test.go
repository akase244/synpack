package main

import "testing"

func TestHasDockerInterfaceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Prefix docker", "docker0", true},
		{"Prefix br-", "br-123abc456def", true},
		{"Prefix veth", "veth0abc1", true},
		{"Prefix tunl", "tunl0", true},
		{"Prefix flannel", "flannel.1", true},
		{"Prefix cni", "cni0", true},
		{"Wired LAN", "eth0", false},
		{"Wireless LAN", "wlp2s0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasDockerInterfaceName(tt.input)
			if result != tt.expected {
				t.Errorf("hasDockerInterfaceName(%q) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}
