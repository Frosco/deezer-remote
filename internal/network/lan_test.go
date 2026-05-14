package network

import (
	"net"
	"testing"
)

func TestIsPrivateIPv4(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.10", true},
		{"10.0.0.1", true},
		{"172.16.5.5", true},
		{"172.31.255.255", true},
		{"172.32.0.0", false},
		{"172.15.0.0", false},
		{"8.8.8.8", false},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, tc := range cases {
		got := IsPrivateIPv4(net.ParseIP(tc.ip))
		if got != tc.want {
			t.Errorf("IsPrivateIPv4(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsVirtualInterface(t *testing.T) {
	virtual := []string{
		"docker0",
		"docker_gwbridge",
		"br-0123456789ab",
		"br-fedcba987654",
		"veth1a2b3c",
		"virbr0",
		"virbr0-nic",
		"vmnet1",
		"vmnet8",
		"vboxnet0",
		"wsl0",
	}
	real := []string{
		"eth0",
		"wlan0",
		"eno1",
		"enp0s31f6",
		"wlp3s0",
		"br0",   // user-configured bridge, not Docker
		"lo",    // separately excluded as loopback
		"wg0",   // wireguard — could be legitimate route
		"tailscale0",
		"tun0",
		"tap0",
	}
	for _, n := range virtual {
		if !isVirtualInterface(n) {
			t.Errorf("isVirtualInterface(%q) = false, want true", n)
		}
	}
	for _, n := range real {
		if isVirtualInterface(n) {
			t.Errorf("isVirtualInterface(%q) = true, want false", n)
		}
	}
}

func TestFilterLANCandidates(t *testing.T) {
	cases := []struct {
		name string
		in   []candidate
		want []string
	}{
		{
			"skips docker bridges and virbr, keeps physical interface",
			[]candidate{
				{name: "wlan0", ip: net.ParseIP("192.168.1.42")},
				{name: "docker0", ip: net.ParseIP("172.17.0.1")},
				{name: "br-0123456789ab", ip: net.ParseIP("172.18.0.1")},
				{name: "br-fedcba987654", ip: net.ParseIP("172.19.0.1")},
				{name: "virbr0", ip: net.ParseIP("192.168.122.1")},
			},
			[]string{"192.168.1.42"},
		},
		{
			"drops non-RFC1918 addresses",
			[]candidate{
				{name: "eth0", ip: net.ParseIP("8.8.8.8")},
				{name: "eth0", ip: net.ParseIP("10.0.0.5")},
			},
			[]string{"10.0.0.5"},
		},
		{
			"keeps bare br0 (user bridge, not Docker pattern)",
			[]candidate{
				{name: "br0", ip: net.ParseIP("192.168.1.10")},
			},
			[]string{"192.168.1.10"},
		},
		{
			"filters veth, vmnet, vboxnet, wsl across mixed candidates",
			[]candidate{
				{name: "veth1a2b", ip: net.ParseIP("10.0.5.1")},
				{name: "vmnet8", ip: net.ParseIP("192.168.140.1")},
				{name: "vboxnet0", ip: net.ParseIP("192.168.56.1")},
				{name: "wsl0", ip: net.ParseIP("172.20.0.1")},
				{name: "eno1", ip: net.ParseIP("10.10.10.10")},
			},
			[]string{"10.10.10.10"},
		},
		{
			"empty when only virtual interfaces have RFC1918 addresses",
			[]candidate{
				{name: "docker0", ip: net.ParseIP("172.17.0.1")},
				{name: "virbr0", ip: net.ParseIP("192.168.122.1")},
			},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterLANCandidates(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d addrs %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for i, ip := range got {
				if ip.String() != tc.want[i] {
					t.Errorf("got[%d] = %s, want %s", i, ip.String(), tc.want[i])
				}
			}
		})
	}
}
