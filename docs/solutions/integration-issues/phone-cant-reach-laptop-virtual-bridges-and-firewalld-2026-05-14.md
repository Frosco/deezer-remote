---
title: "Phone can't reach laptop: virtual bridges shadow real LAN IPs and firewalld silently drops 8080"
date: 2026-05-14
category: integration-issues
module: internal/network
problem_type: integration_issue
component: tooling
severity: high
symptoms:
  - "QR codes scan cleanly but the phone browser hangs on http://172.17.0.1:8080/ and the other 172.x links"
  - "The real Wi-Fi IP is also listed but also times out from the phone"
  - "`deezer-remote doctor` reports every check green, including 'LAN reachable'"
  - "`curl http://<wlan-ip>:8080/` from the laptop itself returns 200"
  - "No log lines server-side when the phone attempts to connect — the SYN never arrives"
root_cause: incomplete_setup
resolution_type: code_fix
tags: [lan-discovery, firewalld, networkmanager, docker-bridge, rfc1918, go, linux]
---

# Phone can't reach laptop: virtual bridges shadow real LAN IPs and firewalld silently drops 8080

## Problem

On Arch / EndeavourOS with Docker, libvirt, and `firewalld` active, the phone could not connect to `deezer-remote serve` via any of the QR codes printed at startup. Pairing was impossible despite the service binding correctly and the laptop sitting on the same Wi-Fi as the phone.

## Symptoms

- QR codes scan cleanly, but the phone browser hangs on `http://172.17.0.1:8080/`, `http://172.18.0.1:8080/`, etc.
- One of the printed URLs (the real Wi-Fi IP) is correct in principle but also times out from the phone.
- `deezer-remote doctor` reports every check green — including "LAN reachable".
- `curl http://<wlan-ip>:8080/` from the same laptop returns `200`.
- Nothing logged server-side when the phone attempts to connect — the SYN never reaches the listener.

## What Didn't Work

- Treating the QR-code list as authoritative and asking the user to "pick the right one". Every IP printed was either host-internal or filtered upstream of the socket.
- Trusting `doctor`'s green output. Its TCP probe runs on the same host as the listener, so `firewalld`'s INPUT chain never sees the packet — local-origin traffic isn't filtered. The check is structurally incapable of detecting an external-reachability problem.
- Looking for an app-level bind bug. `ss -tlnp | grep :8080` showed `*:8080`, so the listener was fine; the problem was upstream of the socket.
- A blanket `firewall-cmd --add-port=8080/tcp --permanent` in zone `public`. That would expose 8080 on every Wi-Fi the laptop ever joins, including coffee shops.

## Solution

Two independent fixes; both are required.

### Fix 1 — filter virtual interfaces in `internal/network/lan.go`

RFC1918 membership alone isn't enough on a typical Linux dev laptop: `docker0`, Docker compose user bridges `br-<hex>`, and libvirt's `virbr0` all hand out private IPs that no phone can reach. Keep the candidate's interface name alongside its IP and filter by name prefix:

```go
var virtualInterfacePrefixes = []string{
    "docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "wsl",
}

type candidate struct {
    name string
    ip   net.IP
}

func isVirtualInterface(name string) bool {
    for _, p := range virtualInterfacePrefixes {
        if strings.HasPrefix(name, p) {
            return true
        }
    }
    return false
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
```

Deliberately *not* filtered (left alone): bare `br0` (user-configured bridge — common in KVM bridged networking where it IS the LAN), `tun*`, `tap*`, `wg*`, `tailscale*`, `zt*`. Any of these can be a legitimate route to the phone.

### Fix 2 — per-SSID `firewalld` zone bound via NetworkManager

A custom zone opens 8080/tcp only on the home Wi-Fi profile. Any other SSID falls back to `public` and 8080 stays closed.

```bash
sudo tee /etc/firewalld/zones/deezer-remote.xml > /dev/null <<'EOF'
<?xml version="1.0" encoding="utf-8"?>
<zone>
  <short>deezer-remote</short>
  <description>Home LAN: public posture plus deezer-remote 8080/tcp</description>
  <service name="ssh"/>
  <service name="dhcpv6-client"/>
  <port port="8080" protocol="tcp"/>
  <forward/>
</zone>
EOF

sudo firewall-cmd --reload
sudo nmcli connection modify "My Home Wi-Fi" connection.zone deezer-remote
sudo nmcli connection up "My Home Wi-Fi"
```

Rejected alternatives, with why:

- **Blanket `firewall-cmd --add-port=8080/tcp` in the default `public` zone** — exposes 8080 on every Wi-Fi the laptop ever joins. Too broad for a tool that holds a Deezer session token in process memory.
- **Source-subnet binding** (`<source address="192.168.10.0/24"/>` on the zone) — fails if a foreign AP also hands out `192.168.10.x` DHCP; peers on that network would also pass.
- **Interface-name binding** (`<interface name="wlan0"/>`) — doesn't distinguish SSIDs. wlan0 is wlan0 whether you're home or at a conference.

## Why This Works

The two failures stack along the path from phone to listener.

The QR list was generated from every up, non-loopback, RFC1918 interface, so the user spent attempts on `docker0` and `virbr0` — gateways the host owns but the phone has no route to. Filtering by interface-name prefix removes those bridges at the source. Once only the Wi-Fi IP is printed, the kernel correctly forwards the SYN up the stack — and that's where `firewalld`'s `public` zone (which permits only `ssh` and `dhcpv6-client` by default) silently drops it. Binding a custom zone to the NetworkManager *connection* (not the interface) means the rule activates exactly when the home SSID is in use and disappears on any other Wi-Fi — the correct safety posture for a single-user LAN service.

## Prevention

- **Generalizable pattern: RFC1918 ≠ phone-reachable.** When advertising "LAN addresses" from Go on Linux, combine the IP-range check with an interface-name prefix filter for known host-internal bridge namespaces (`docker`, `br-`, `veth`, `virbr`, `vmnet`, `vboxnet`, `wsl`). Keep the allowed-by-default set (`tun*`, `tap*`, `wg*`, `tailscale*`, `zt*`, bare `br0`) explicit so future additions are deliberate.
- **Tests:** unit-test `filterLANCandidates` with synthetic `(name, ip)` candidates covering each virtual prefix, a real wlan/eth case, and the deliberately-allowed overlay cases. The function is pure once the candidate list is materialized — no `net.Interfaces()` in the test path. See `internal/network/lan_test.go::TestFilterLANCandidates` and `TestIsVirtualInterface`.
- **`doctor` enhancement candidate (open follow-up):** detect `firewalld` active (`systemctl is-active firewalld`), enumerate the active NM connection's zone, and warn if `8080/tcp` is not permitted there. The current "LAN reachable from self" probe cannot catch this because local-origin traffic bypasses the INPUT chain. Any real reachability check needs an out-of-host vantage point (e.g., instruct the user to curl from the phone, or bind-test from a non-loopback address with `SO_BINDTODEVICE`).
- **README posture:** keep the per-SSID `firewalld` recipe in the project README so the next install doesn't rediscover this from scratch. Mention the rejected alternatives so a reader knows why the per-SSID NM zone is the chosen shape.

## Related

- Background on the original (insufficient) RFC1918-only filter that this learning supersedes: [`docs/superpowers/plans/2026-05-14-deezer-remote-phase1.md`](../../superpowers/plans/2026-05-14-deezer-remote-phase1.md) — see the `internal/network` task and the `LANAddrs` definition.
- Locked design context for pairing / discovery: [`docs/superpowers/specs/2026-05-13-deezer-remote-design.md`](../../superpowers/specs/2026-05-13-deezer-remote-design.md) — step 2 of first-run pairing.
- Sibling Go gotcha from the same project: [`docs/solutions/conventions/go-method-names-vs-stdlib-interfaces-2026-05-14.md`](../conventions/go-method-names-vs-stdlib-interfaces-2026-05-14.md).
- Commit: `d20dfb7` — `fix(network): skip host-internal bridges in LAN discovery`.
