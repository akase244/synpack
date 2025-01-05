package main

import "testing"

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
			t.Errorf("hasDockerInterfaceName(%q) = %v; want %v", test.input, acutual, test.expect)
		}
	}
}
