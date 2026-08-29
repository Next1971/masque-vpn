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
