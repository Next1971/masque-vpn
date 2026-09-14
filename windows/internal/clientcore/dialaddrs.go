package clientcore

import (
	"fmt"
	"net"
	"sort"
	"strconv"
)

// resolveDialAddrs turns [server].server plus optional alt_port into concrete
// UDP endpoints. A hostname yields every A and AAAA; IPv6 is listed first so
// a single-address path prefers QUIC over IPv6 when DNS has AAAA.
func resolveDialAddrs(server string, altPort int) ([]*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(server)
	if err != nil {
		return nil, fmt.Errorf("server %q: %w", server, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("server %q: invalid port", server)
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("lookup %q: %w", host, err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses for %q", host)
	}

	sort.SliceStable(ips, func(i, j int) bool {
		return ipIs6(ips[i]) && !ipIs6(ips[j])
	})

	ports := []int{port}
	if altPort >= 1 && altPort <= 65535 && altPort != port {
		ports = append(ports, altPort)
	}

	seen := make(map[string]struct{})
	var out []*net.UDPAddr
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		for _, p := range ports {
			a := &net.UDPAddr{IP: ip, Port: p}
			key := a.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no dial addresses for %q", server)
	}
	return out, nil
}

func ipIs6(ip net.IP) bool {
	return ip != nil && ip.To4() == nil && ip.To16() != nil
}

// listenUDPFor opens a UDP socket in the same address family as dst.
// An IPv4-only wildcard cannot send to an IPv6 QUIC peer.
func listenUDPFor(dst *net.UDPAddr) (*net.UDPConn, error) {
	if dst == nil || dst.IP == nil {
		return nil, fmt.Errorf("listen UDP: missing destination")
	}
	if dst.IP.To4() != nil {
		return net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	}
	return net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified})
}

// ServerHost is the host part of a profile or DialAddr (IPv6-safe).
func ServerHost(server string) string {
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		return server
	}
	return host
}
