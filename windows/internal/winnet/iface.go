//go:build windows

package winnet

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

const tunnelSubnetBits = 24

func tunnelGateway(client netip.Addr) netip.Addr {
	if !client.Is4() {
		return client
	}
	p := netip.PrefixFrom(client, tunnelSubnetBits).Masked()
	b := p.Addr().As4()
	b[3]++
	return netip.AddrFrom4(b)
}

func tunnelGateway6(client netip.Addr) netip.Addr {
	p := netip.PrefixFrom(client, 64).Masked()
	return p.Addr().Next()
}

func IfUpIPv6(iface string, addr netip.Prefix) error {
	cidr := netip.PrefixFrom(addr.Addr(), 64).String()
	if err := runCmd("netsh", "interface", "ipv6", "add", "address", iface, cidr); err != nil {
		return err
	}
	_ = runCmd("netsh", "interface", "ipv6", "set", "subinterface", iface, "mtu=1400", "store=active")
	return nil
}

// SetupIPv6Default sends IPv6 internet traffic through TUN via the ULA gateway (::1 in /64).
func SetupIPv6Default(iface string, client netip.Addr) (func(), error) {
	gw := tunnelGateway6(client)
	if err := runCmd("netsh", "interface", "ipv6", "add", "route", "::/0", iface, gw.String()); err != nil {
		return nil, err
	}
	return func() {
		if err := runCmd("netsh", "interface", "ipv6", "delete", "route", "::/0", iface, gw.String()); err != nil {
			log.Printf("cleanup: del IPv6 default: %v", err)
		}
	}, nil
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IfUp assigns the client address to the Wintun adapter (/24 on-link mask).
func IfUp(iface string, addr netip.Prefix) error {
	ip := addr.Addr().String()
	mask := prefixToMask(tunnelSubnetBits)
	if err := runCmd("netsh", "interface", "ip", "set", "address",
		"name="+iface, "static", ip, mask); err != nil {
		return err
	}
	_ = runCmd("netsh", "interface", "ipv4", "set", "subinterface", iface, "mtu=1400", "store=active")
	return nil
}

// SetupFullRoute sends all IPv4 traffic through TUN and sets tunnel DNS.
func SetupFullRoute(iface, server string, client netip.Addr, dns []string) (func(), error) {
	host := server
	if i := strings.LastIndex(server, ":"); i > 0 {
		host = server[:i]
	}
	serverIP, err := netip.ParseAddr(host)
	if err != nil {
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
	tunGW := tunnelGateway(client)

	srvStr := serverIP.String()
	srvArgs := []string{"add", srvStr, "mask", "255.255.255.255", gw.String(), "metric", "1"}
	if gwIdx > 0 {
		srvArgs = append(srvArgs, "if", strconv.Itoa(gwIdx))
	}
	if err := runCmd("route", srvArgs...); err != nil {
		return nil, fmt.Errorf("add server bypass route: %w", err)
	}

	added := [][2]string{}
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

func ifIndex(name string) (int, error) {
	out, err := exec.Command("netsh", "interface", "ipv4", "show", "interfaces").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("show interfaces: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
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

func defaultGatewayWin() (netip.Addr, int, error) {
	out, err := exec.Command("route", "print", "-4").CombinedOutput()
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("route print: %w", err)
	}
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
	return bestGW, 0, nil
}

// RunPingTest is kept for the standalone console client.
func RunPingTest(ctx context.Context, dst string, count int) error {
	out, err := exec.CommandContext(ctx, "ping", "-n", strconv.Itoa(count), "-w", "5000", dst).CombinedOutput()
	log.Printf("ping output:\n%s", strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	if !strings.Contains(string(out), "TTL=") && !strings.Contains(string(out), "ttl=") {
		return fmt.Errorf("no reply (no TTL in output)")
	}
	return nil
}
