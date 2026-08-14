//go:build linux

package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"strings"
)

// runCmd runs a command and returns an error with output on failure.
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ifUp brings up the interface with the client address (use /32 because the address is point-to-point).
func ifUp(iface string, addr netip.Prefix) error {
	// The address is assigned as /32—assign it to the interface as /32 and bring the link up.
	if err := runCmd("ip", "addr", "add", addr.String(), "dev", iface); err != nil {
		return err
	}
	if err := runCmd("ip", "link", "set", "dev", iface, "up"); err != nil {
		return err
	}
	return nil
}

// setupTestRoute adds a route ONLY to dst through TUN without changing the default route.
// Set src to the client tunnel address so outgoing packets have the
// correct source (otherwise the kernel NATs a foreign source and the reply does not return).
// Safe on a VPS: SSH and server traffic continue using the previous path.
// Returns cleanup, which removes the route.
func setupTestRoute(iface string, dst netip.Addr, src netip.Addr) (func(), error) {
	route := dst.String() + "/32"
	if err := runCmd("ip", "route", "add", route, "dev", iface, "src", src.String()); err != nil {
		return nil, err
	}
	return func() {
		if err := runCmd("ip", "route", "del", route, "dev", iface); err != nil {
			log.Printf("cleanup: del route %s: %v", route, err)
		}
	}, nil
}

// setupFullRoute routes all traffic through TUN. To prevent QUIC packets to the VPS
// from looping into the tunnel, it adds a host route to the server through the current
// default gateway. For a real device only (E3), NOT a VPS.
func setupFullRoute(iface, server string, _ netip.Addr, _ []string) (func(), error) {
	// Extract the server IP (host:port).
	host := server
	if i := strings.LastIndex(server, ":"); i > 0 {
		host = server[:i]
	}
	serverIP, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("server host %q is not an IP (test mode expects literal IP): %w", host, err)
	}

	// Determine the current default gateway.
	gw, dev, err := defaultGateway()
	if err != nil {
		return nil, fmt.Errorf("detect default gateway: %w", err)
	}
	log.Printf("current default gateway: %s dev %s", gw, dev)

	// 1. Host route to the VPS through the previous gateway (otherwise it loops).
	srvRoute := serverIP.String() + "/32"
	if err := runCmd("ip", "route", "add", srvRoute, "via", gw.String(), "dev", dev); err != nil {
		return nil, fmt.Errorf("add server bypass route: %w", err)
	}

	// 2. Route all traffic through TUN using two /1 halves (they override the default
	//    without removing the original, so rollback is easy).
	added := []string{}
	for _, half := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if err := runCmd("ip", "route", "add", half, "dev", iface); err != nil {
			// roll back routes already added
			for _, h := range added {
				_ = runCmd("ip", "route", "del", h, "dev", iface)
			}
			_ = runCmd("ip", "route", "del", srvRoute, "via", gw.String(), "dev", dev)
			return nil, fmt.Errorf("add default-half %s: %w", half, err)
		}
		added = append(added, half)
	}

	return func() {
		for _, h := range added {
			if err := runCmd("ip", "route", "del", h, "dev", iface); err != nil {
				log.Printf("cleanup: del %s: %v", h, err)
			}
		}
		if err := runCmd("ip", "route", "del", srvRoute, "via", gw.String(), "dev", dev); err != nil {
			log.Printf("cleanup: del server route: %v", err)
		}
	}, nil
}

// defaultGateway parses `ip route show default` → (gateway, dev).
func defaultGateway() (netip.Addr, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		return netip.Addr{}, "", fmt.Errorf("ip route show default: %w", err)
	}
	// example: "default via 203.0.113.1 dev ens3 proto static"
	fields := strings.Fields(string(out))
	var gw, dev string
	for i := 0; i < len(fields)-1; i++ {
		switch fields[i] {
		case "via":
			gw = fields[i+1]
		case "dev":
			dev = fields[i+1]
		}
	}
	if gw == "" || dev == "" {
		return netip.Addr{}, "", fmt.Errorf("could not parse default route: %q", strings.TrimSpace(string(out)))
	}
	addr, err := netip.ParseAddr(gw)
	if err != nil {
		return netip.Addr{}, "", fmt.Errorf("parse gateway %q: %w", gw, err)
	}
	return addr, dev, nil
}

// runPingTest sends ICMP echo using the system ping command through the configured route,
// binding to client TUN (-I iface) so the source is the tunnel address.
// Packets travel TUN → kernel → server → Internet → back. It tests the data plane.
func runPingTest(ctx context.Context, dst, iface string, count int) error {
	log.Printf("sending %d ICMP echo(s) to %s via tunnel (bind %s)...", count, dst, iface)
	out, err := exec.CommandContext(ctx, "ping", "-c", fmt.Sprint(count), "-W", "5", "-I", iface, dst).CombinedOutput()
	log.Printf("ping output:\n%s", strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	log.Printf("✅ ping through tunnel SUCCEEDED — client core data-plane WORKS")
	return nil
}
