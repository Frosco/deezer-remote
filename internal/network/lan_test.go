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

func TestLANAddrs_FiltersToPrivate(t *testing.T) {
	in := []net.IP{
		net.ParseIP("192.168.1.10"),
		net.ParseIP("127.0.0.1"),
		net.ParseIP("fe80::1"),
		net.ParseIP("10.0.0.5"),
		net.ParseIP("8.8.8.8"),
	}
	got := filterPrivateIPv4(in)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %v", len(got), got)
	}
	if got[0].String() != "192.168.1.10" || got[1].String() != "10.0.0.5" {
		t.Errorf("filter = %v", got)
	}
}
