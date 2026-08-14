//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
)

// tunnelSubnetBits is the tunnel subnet mask (server pool 10.8.0.0/24).
// The client address is assigned to the interface with this mask so the whole subnet
// is on-link and next-hop 10.8.0.1 is reachable as a normal gateway.
const tunnelSubnetBits = 24

// tunnelGateway returns the server address in the tunnel = the first address of the
// client subnet (e.g. 10.8.0.254/24 → 10.8.0.1). The server reserves .1.
func tunnelGateway(client netip.Addr) netip.Addr {
	if !client.Is4() {
		return client
	}
	p := netip.PrefixFrom(client, tunnelSubnetBits).Masked()
	// first host in subnet = network + 1
	b := p.Addr().As4()
	b[3]++
	return netip.AddrFrom4(b)
}

// runCmd runs a command and returns an error with output on failure.
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ifUp assigns the client address to the Wintun adapter and brings it up.
// On Windows, use netsh; the mask for a /32 tunnel address is 255.255.255.255,
// but netsh accepts that address as an interface host address. To ensure the system
// correctly builds on-link routes into TUN, set the address using the assigned
// prefix mask (usually /32) and explicitly add the subnet by routes later.
func ifUp(iface string, addr netip.Prefix) error {
	ip := addr.Addr().String()
	// IMPORTANT: the server assigns an address as /32. If /32 is assigned to the interface,
	// it has no on-link subnet, and Windows interprets a route to dst through “gateway = its own
	// address” as single-hop → TTL=1 → connect-ip-go
	// drops packets (“Hop Limit too small: 1”). Therefore, assign the address
	// with mask /24 (the full 10.8.0.0/24 pool becomes on-link), and send traffic to dst
	// through gateway 10.8.0.1 (the server tunnel address)—a normal
	// gateway hop with a normal TTL.
	mask := prefixToMask(tunnelSubnetBits)
	// netsh interface ip set address name="<iface>" static <ip> <mask>
	if err := runCmd("netsh", "interface", "ip", "set", "address",
		"name="+iface, "static", ip, mask); err != nil {
		return err
	}
	// MTU is set by CreateTUN; set it through netsh as an additional safeguard.
	// (not critical; do not treat an error as fatal)
	_ = runCmd("netsh", "interface", "ipv4", "set", "subinterface", iface, "mtu=1400", "store=active")
	return nil
}

// setupTestRoute adds a route ONLY to dst through TUN without changing the default route.
// On Windows: route add <dst> mask 255.255.255.255 <gateway> if <ifindex>.
// Gateway = the server tunnel address (the first client-subnet address, e.g.
// 10.8.0.1). This is a REAL next hop within the on-link /24 subnet, so
// Windows creates packets with a normal TTL (not single-hop), and connect-ip-go
// proxies them instead of dropping them as “Hop Limit too small: 1”.
func setupTestRoute(iface string, dst netip.Addr, src netip.Addr) (func(), error) {
	idx, err := ifIndex(iface)
	if err != nil {
		return nil, err
	}
	gw := tunnelGateway(src)
	dstStr := dst.String()
	// route add 1.1.1.1 mask 255.255.255.255 <tunnel-gw=10.8.0.1> metric 1 if <idx>
	if err := runCmd("route", "add", dstStr, "mask", "255.255.255.255",
		gw.String(), "metric", "1", "if", strconv.Itoa(idx)); err != nil {
		return nil, err
	}
	return func() {
		if err := runCmd("route", "delete", dstStr); err != nil {
			log.Printf("cleanup: route delete %s: %v", dstStr, err)
		}
	}, nil
}

// setupFullRoute routes all traffic through TUN. To prevent QUIC packets to the VPS
// from looping, it adds a host route to the server through the current default gateway.
// It then adds two /1 halves that override the default route (and are easy to roll back).
func setupFullRoute(iface, server string, client netip.Addr, dns []string) (func(), error) {
	host := server
	if i := strings.LastIndex(server, ":"); i > 0 {
		host = server[:i]
	}
	serverIP, err := netip.ParseAddr(host)
	if err != nil {
		// host is a name rather than an IP: resolve it to IPv4 for the VPS bypass route.
		ips, rerr := net.LookupIP(host)
		if rerr != nil {
			return nil, fmt.Errorf("resolve server host %q: %w", host, rerr)
		}
		for _, ip := range ips {
			if v4 := ip.To4(); v4 != nil {
				if a, ok := netip.AddrFromSlice(v4); ok {
					serverIP = a
					break
				}
			}
		}
		if !serverIP.IsValid() {
			return nil, fmt.Errorf("no IPv4 address for server host %q", host)
		}
		log.Printf("resolved server %s → %s (for bypass route)", host, serverIP)
	}

	gw, gwIdx, err := defaultGatewayWin()
	if err != nil {
		return nil, fmt.Errorf("detect default gateway: %w", err)
	}
	log.Printf("current default gateway: %s (if %d)", gw, gwIdx)

	idx, err := ifIndex(iface)
	if err != nil {
		return nil, err
	}
	// next hop in the tunnel (10.8.0.1), so packets use a normal TTL
	// rather than single-hop TTL=1 (see setupTestRoute comment).
	// Take the address from the server-assigned value (client), rather than querying the
	// interface, to avoid a Windows/Wintun race (the address may not be applied yet).
	tunGW := tunnelGateway(client)

	// 1. Host route to the VPS through the previous gateway (otherwise it loops).
	// If gwIdx==0, do not specify “if”; route selects the interface by gateway.
	srvStr := serverIP.String()
	srvArgs := []string{"add", srvStr, "mask", "255.255.255.255", gw.String(), "metric", "1"}
	if gwIdx > 0 {
		srvArgs = append(srvArgs, "if", strconv.Itoa(gwIdx))
	}
	if err := runCmd("route", srvArgs...); err != nil {
		return nil, fmt.Errorf("add server bypass route: %w", err)
	}

	// 2. Two default-route halves through TUN.
	added := [][2]string{} // {network, mask}
	halves := [][2]string{{"0.0.0.0", "128.0.0.0"}, {"128.0.0.0", "128.0.0.0"}}
	for _, h := range halves {
		if err := runCmd("route", "add", h[0], "mask", h[1],
			tunGW.String(), "metric", "1", "if", strconv.Itoa(idx)); err != nil {
			for _, a := range added {
				_ = runCmd("route", "delete", a[0], "mask", a[1])
			}
			_ = runCmd("route", "delete", srvStr)
			return nil, fmt.Errorf("add default-half %s: %w", h[0], err)
		}
		added = append(added, h)
	}

	// 3. DNS on the tunnel interface. CONNECT-IP does not advertise DNS
	// (not in RFC 9484), so use the profile values and set them on masque0.
	// Traffic to DNS:53 goes through the tunnel (the default route is already redirected).
	dnsSet := false
	for i, d := range dns {
		if i == 0 {
			if err := runCmd("netsh", "interface", "ip", "set", "dns",
				"name="+iface, "static", d, "primary"); err != nil {
				log.Printf("warn: set primary DNS %s on %s: %v", d, iface, err)
			} else {
				dnsSet = true
				log.Printf("DNS %s set on %s (tunnel)", d, iface)
			}
		} else {
			if err := runCmd("netsh", "interface", "ip", "add", "dns",
				"name="+iface, d, "index="+strconv.Itoa(i+1)); err != nil {
				log.Printf("warn: add DNS %s on %s: %v", d, iface, err)
			}
		}
	}

	return func() {
		if dnsSet {
			// Restore automatic DNS (DHCP) on the tunnel interface.
			// (masque0 is removed when TUN is closed in any case.)
			if err := runCmd("netsh", "interface", "ip", "set", "dns",
				"name="+iface, "dhcp"); err != nil {
				log.Printf("cleanup: reset DNS on %s: %v", iface, err)
			}
		}
		for _, a := range added {
			if err := runCmd("route", "delete", a[0], "mask", a[1]); err != nil {
				log.Printf("cleanup: del %s: %v", a[0], err)
			}
		}
		if err := runCmd("route", "delete", srvStr); err != nil {
			log.Printf("cleanup: del server route: %v", err)
		}
	}, nil
}

// runPingTest sends ICMP echo through the configured route. On Windows,
// ping cannot be bound to an interface (-I), but the route to dst is already directed
// into TUN with the required src, so packets travel through the tunnel.
func runPingTest(ctx context.Context, dst, iface string, count int) error {
	log.Printf("sending %d ICMP echo(s) to %s via tunnel...", count, dst)
	// ping -n <count> -w 5000 <dst>
	out, err := exec.CommandContext(ctx, "ping", "-n", strconv.Itoa(count), "-w", "5000", dst).CombinedOutput()
	// Windows ping outputs in OEM encoding; print it as-is.
	log.Printf("ping output:\n%s", strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	// On some Windows versions, ping returns 0 even with partial loss, so
	// additionally check for “TTL=” (an indication of a successful reply).
	if !strings.Contains(string(out), "TTL=") && !strings.Contains(string(out), "ttl=") {
		return fmt.Errorf("no reply (no TTL in output)")
	}
	log.Printf("ping through tunnel SUCCEEDED — client core data-plane WORKS")
	return nil
}

// --- Windows helper functions ---

// prefixToMask converts a prefix length to a mask such as 255.255.255.255.
func prefixToMask(bits int) string {
	if bits < 0 || bits > 32 {
		bits = 32
	}
	var m uint32 = 0xffffffff << (32 - bits)
	if bits == 0 {
		m = 0
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(m>>24), byte(m>>16), byte(m>>8), byte(m))
}

// ifIndex returns the numeric interface index by name through
// `netsh interface ipv4 show interfaces`.
func ifIndex(name string) (int, error) {
	out, err := exec.Command("netsh", "interface", "ipv4", "show", "interfaces").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("show interfaces: %w", err)
	}
	// Lines such as: "  Idx     Met         MTU          State                Name"
	// followed by: "   23        25        1400  connected            masque0"
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		// The name can contain spaces—take the tail after the fourth field.
		// Build the name from everything starting with the fifth field.
		idxStr := f[0]
		nm := strings.TrimSpace(strings.Join(f[4:], " "))
		if nm == name {
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				continue
			}
			return idx, nil
		}
	}
	return 0, fmt.Errorf("interface %q not found in netsh output", name)
}

// ifAddr returns the first IPv4 address of an interface (for use as an on-link gateway).
func ifAddr(name string) (netip.Addr, error) {
	out, err := exec.Command("netsh", "interface", "ipv4", "show", "addresses", "name="+name).CombinedOutput()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("show addresses: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// "IP Address:                           10.8.0.254"
		if i := strings.LastIndex(line, ":"); i >= 0 && strings.Contains(strings.ToLower(line), "address") {
			cand := strings.TrimSpace(line[i+1:])
			if a, err := netip.ParseAddr(cand); err == nil && a.Is4() {
				return a, nil
			}
		}
	}
	return netip.Addr{}, fmt.Errorf("no IPv4 address on %q", name)
}

// defaultGatewayWin parses the default gateway and its interface index from `route print -4`.
func defaultGatewayWin() (netip.Addr, int, error) {
	out, err := exec.Command("route", "print", "-4").CombinedOutput()
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("route print: %w", err)
	}
	// Look for the line "0.0.0.0    0.0.0.0    <gateway>    <iface-ip>    <metric>"
	var bestGW netip.Addr
	bestMetric := int(^uint(0) >> 1)
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[0] == "0.0.0.0" && f[1] == "0.0.0.0" {
			gw, err := netip.ParseAddr(f[2])
			if err != nil {
				continue
			}
			metric, _ := strconv.Atoi(f[len(f)-1])
			if metric < bestMetric {
				bestMetric = metric
				bestGW = gw
			}
		}
	}
	if !bestGW.IsValid() {
		return netip.Addr{}, 0, fmt.Errorf("default gateway not found in route print")
	}
	// Determine the gateway interface index from the interface through which it is reachable:
	// use the `route print` interface list—but it is simpler to get the index by gateway IP
	// through `netsh interface ipv4 show route`. The gateway is sufficient for the bypass;
	// use the index of an interface on the same subnet. Simplify this:
	// return 0 and do not specify “if”—route selects it by gateway.
	return bestGW, 0, nil
}
