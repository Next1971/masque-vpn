package vpssetup

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// CandidatePorts are the naive UDP suggestions (local bind check only).
var CandidatePorts = []int{443, 2053, 8443, 41234}

// ParseListeningUDPPorts reads `ss -H -uln` (or similar) local addresses and
// returns the set of UDP ports bound on this host.
func ParseListeningUDPPorts(ssOutput string) map[int]struct{} {
	out := make(map[int]struct{})
	sc := bufio.NewScanner(strings.NewReader(ssOutput))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Typical: UNCONN 0 0 0.0.0.0:443 ...  or [::]:443
		fields := strings.Fields(line)
		for _, f := range fields {
			p, ok := portFromAddr(f)
			if ok {
				out[p] = struct{}{}
				break
			}
		}
	}
	return out
}

func portFromAddr(f string) (int, bool) {
	// Strip zone / trailing commas.
	f = strings.TrimSuffix(f, ",")
	i := strings.LastIndex(f, ":")
	if i < 0 || i == len(f)-1 {
		return 0, false
	}
	ps := f[i+1:]
	n, err := strconv.Atoi(ps)
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// Recommend returns candidate ports that are not already bound, in preference order.
func Recommend(listening map[int]struct{}) []int {
	var rec []int
	for _, p := range CandidatePorts {
		if _, taken := listening[p]; !taken {
			rec = append(rec, p)
		}
	}
	return rec
}

// RecommendFree returns candidates where neither UDP nor TCP is listening.
func RecommendFree(udp, tcp map[int]struct{}) []int {
	var rec []int
	for _, p := range CandidatePorts {
		if _, taken := udp[p]; taken {
			continue
		}
		if _, taken := tcp[p]; taken {
			continue
		}
		rec = append(rec, p)
	}
	return rec
}

// DescribeConflicts explains listen/alternate ports that are already taken.
// A listen UDP conflict means the service cannot bind. A listen TCP conflict
// leaves UDP up and skips TCP fallback. An alternate-port conflict means
// continuing would redirect that traffic to the listen port.
func DescribeConflicts(udp, tcp map[int]struct{}, listen, alt int) []string {
	lines := conflictLines(udp, tcp, listen, false)
	if alt > 0 && alt != listen {
		lines = append(lines, conflictLines(udp, tcp, alt, true)...)
	}
	return lines
}

func conflictLines(udp, tcp map[int]struct{}, port int, redirect bool) []string {
	_, u := udp[port]
	_, t := tcp[port]
	if !u && !t {
		return nil
	}
	if redirect {
		var what []string
		if u {
			what = append(what, "UDP")
		}
		if t {
			what = append(what, "TCP")
		}
		return []string{fmt.Sprintf("%s %d is already in use. Continuing redirects that traffic to the listen port.", strings.Join(what, " and "), port)}
	}
	var lines []string
	if u {
		lines = append(lines, fmt.Sprintf("UDP %d is already listening. MASQUE cannot bind it, so the service will not start.", port))
	}
	if t {
		lines = append(lines, fmt.Sprintf("TCP %d is already listening. Continuing leaves UDP up and does not open TCP fallback on this port.", port))
	}
	return lines
}

func formatPortList(ports []int) string {
	if len(ports) == 0 {
		return "(none)"
	}
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(parts, ", ")
}
