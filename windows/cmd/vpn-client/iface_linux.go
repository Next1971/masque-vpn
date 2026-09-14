//go:build linux

package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"strings"

	"masque-client/internal/clientcore"
)

// runCmd runs a command and returns an error with output on failure.
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ifUp(iface string, addr netip.Prefix) error {
	if err := runCmd("ip", "addr", "add", addr.String(), "dev", iface); err != nil {
		return err
	}
	if err := runCmd("ip", "link", "set", "dev", iface, "up"); err != nil {
		return err
	}
	return nil
}

func ifUpIPv6(iface string, addr netip.Prefix) error {
	cidr := netip.PrefixFrom(addr.Addr(), 64).String()
	return runCmd("ip", "addr", "add", cidr, "dev", iface)
}

func setupIPv6Default(iface string, client netip.Addr) (func(), error) {
	args := []string{"-6", "route", "add", "::/0", "dev", iface}
	if client.Is6() {
		args = append(args, "src", client.String())
	}
	if err := runCmd("ip", args...); err != nil {
		return nil, err
	}
	return func() {
		if err := runCmd("ip", "-6", "route", "del", "::/0", "dev", iface); err != nil {
			log.Printf("cleanup: del ::/0: %v", err)
		}
	}, nil
}

func setupTestRoute(iface string, dst netip.Addr, src netip.Addr) (func(), error) {
	fam := []string{}
	route := dst.String() + "/32"
	if dst.Is6() {
		fam = []string{"-6"}
		route = dst.String() + "/128"
	}
	args := append(append([]string{}, fam...), "route", "add", route, "dev", iface, "src", src.String())
	if err := runCmd("ip", args...); err != nil {
		return nil, err
	}
	return func() {
		del := append(append([]string{}, fam...), "route", "del", route, "dev", iface)
		if err := runCmd("ip", del...); err != nil {
			log.Printf("cleanup: del route %s: %v", route, err)
		}
	}, nil
}

func setupFullRoute(iface, server string, _ netip.Addr, _ []string) (func(), error) {
	host := clientcore.ServerHost(server)
	serverIP, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("server host %q is not an IP (expects literal IP): %w", host, err)
	}

	bypassClean, err := addServerBypass(serverIP)
	if err != nil {
		return nil, err
	}

	added := []string{}
	for _, half := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if err := runCmd("ip", "route", "add", half, "dev", iface); err != nil {
			for _, h := range added {
				_ = runCmd("ip", "route", "del", h, "dev", iface)
			}
			bypassClean()
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
		bypassClean()
	}, nil
}

func addServerBypass(serverIP netip.Addr) (func(), error) {
	if serverIP.Is6() {
		gw, dev, err := defaultGateway6()
		if err != nil {
			return nil, fmt.Errorf("detect default IPv6 gateway: %w", err)
		}
		log.Printf("current default IPv6 gateway: %s dev %s", gw, dev)
		srvRoute := serverIP.String() + "/128"
		if err := runCmd("ip", "-6", "route", "add", srvRoute, "via", gw.String(), "dev", dev); err != nil {
			return nil, fmt.Errorf("add server IPv6 bypass route: %w", err)
		}
		return func() {
			if err := runCmd("ip", "-6", "route", "del", srvRoute, "via", gw.String(), "dev", dev); err != nil {
				log.Printf("cleanup: del server IPv6 route: %v", err)
			}
		}, nil
	}

	gw, dev, err := defaultGateway()
	if err != nil {
		return nil, fmt.Errorf("detect default gateway: %w", err)
	}
	log.Printf("current default gateway: %s dev %s", gw, dev)
	srvRoute := serverIP.String() + "/32"
	if err := runCmd("ip", "route", "add", srvRoute, "via", gw.String(), "dev", dev); err != nil {
		return nil, fmt.Errorf("add server bypass route: %w", err)
	}
	return func() {
		if err := runCmd("ip", "route", "del", srvRoute, "via", gw.String(), "dev", dev); err != nil {
			log.Printf("cleanup: del server route: %v", err)
		}
	}, nil
}

func defaultGateway6() (netip.Addr, string, error) {
	out, err := exec.Command("ip", "-6", "route", "show", "default").CombinedOutput()
	if err != nil {
		return netip.Addr{}, "", fmt.Errorf("ip -6 route show default: %w", err)
	}
	return parseDefaultRoute(string(out))
}

func defaultGateway() (netip.Addr, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		return netip.Addr{}, "", fmt.Errorf("ip route show default: %w", err)
	}
	return parseDefaultRoute(string(out))
}

func parseDefaultRoute(out string) (netip.Addr, string, error) {
	fields := strings.Fields(out)
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
		return netip.Addr{}, "", fmt.Errorf("could not parse default route: %q", strings.TrimSpace(out))
	}
	addr, err := netip.ParseAddr(gw)
	if err != nil {
		return netip.Addr{}, "", fmt.Errorf("parse gateway %q: %w", gw, err)
	}
	return addr, dev, nil
}

func runPingTest(ctx context.Context, dst, iface string, count int) error {
	log.Printf("sending %d ICMP echo(s) to %s via tunnel (bind %s)...", count, dst, iface)
	args := []string{"-c", fmt.Sprint(count), "-W", "5", "-I", iface, dst}
	if strings.Contains(dst, ":") {
		args = append([]string{"-6"}, args...)
	}
	out, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	log.Printf("ping output:\n%s", strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	log.Printf("✅ ping through tunnel SUCCEEDED — client core data-plane WORKS")
	return nil
}
