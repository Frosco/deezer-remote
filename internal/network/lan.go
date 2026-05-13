// Package network exposes LAN-discovery helpers for the serve / doctor
// commands.
package network

import (
	"net"
)

var (
	cidr10  = mustCIDR("10.0.0.0/8")
	cidr172 = mustCIDR("172.16.0.0/12")
	cidr192 = mustCIDR("192.168.0.0/16")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// IsPrivateIPv4 reports whether ip is in an RFC1918 range.
func IsPrivateIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return cidr10.Contains(v4) || cidr172.Contains(v4) || cidr192.Contains(v4)
}

// LANAddrs returns every interface IPv4 address in an RFC1918 range, sorted
// in the order the OS returns them.
func LANAddrs() ([]net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var all []net.IP
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil {
				all = append(all, ip)
			}
		}
	}
	return filterPrivateIPv4(all), nil
}

func filterPrivateIPv4(in []net.IP) []net.IP {
	var out []net.IP
	for _, ip := range in {
		if IsPrivateIPv4(ip) {
			out = append(out, ip.To4())
		}
	}
	return out
}
