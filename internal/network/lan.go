// Package network exposes LAN-discovery helpers for the serve / doctor
// commands.
package network

import (
	"net"
	"strings"
)

var (
	cidr10  = mustCIDR("10.0.0.0/8")
	cidr172 = mustCIDR("172.16.0.0/12")
	cidr192 = mustCIDR("192.168.0.0/16")
)

// virtualInterfacePrefixes lists name prefixes for host-internal virtual
// bridges that hand out RFC1918 addresses but are not reachable from other
// devices on the LAN. The list is intentionally conservative — it covers the
// usual suspects (Docker, libvirt, VMware, VirtualBox, WSL) without touching
// VPN tunnels (tun/tap/wg/tailscale/zt) or user-configured bridges (bare br0),
// which can legitimately be the route to the phone.
var virtualInterfacePrefixes = []string{
	"docker",
	"br-",
	"veth",
	"virbr",
	"vmnet",
	"vboxnet",
	"wsl",
}

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

// candidate pairs an interface name with one of its IPv4 addresses, so that
// filtering can drop host-internal virtual bridges by name even when their
// addresses look like ordinary LAN IPs.
type candidate struct {
	name string
	ip   net.IP
}

// isVirtualInterface returns true for interface names that match a known
// host-internal virtual bridge prefix.
func isVirtualInterface(name string) bool {
	for _, p := range virtualInterfacePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// LANAddrs returns every interface IPv4 address that is in an RFC1918 range
// and not on a known host-internal virtual bridge (docker0, br-*, virbr0,
// vboxnet*, vmnet*, wsl*, veth*). The order matches what the OS returns.
func LANAddrs() ([]net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var cands []candidate
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
				cands = append(cands, candidate{name: ifc.Name, ip: ip})
			}
		}
	}
	return filterLANCandidates(cands), nil
}

func filterLANCandidates(in []candidate) []net.IP {
	var out []net.IP
	for _, c := range in {
		if !IsPrivateIPv4(c.ip) {
			continue
		}
		if isVirtualInterface(c.name) {
			continue
		}
		out = append(out, c.ip.To4())
	}
	return out
}
