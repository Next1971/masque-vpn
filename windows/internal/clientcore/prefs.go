package clientcore

import (
	"strings"
	"time"
)

// TransportMode normalizes the app setting.
// "auto" (also empty) probes UDP and falls back to TCP.
// "udp" never opens TCP. "tcp" never opens UDP.
func TransportMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "udp", "tcp":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "auto"
	}
}

// ClampMTU keeps a user MTU inside 1280–1500. Anything else becomes TunnelMTU.
func ClampMTU(v int) int {
	if v < 1280 || v > 1500 {
		return TunnelMTU
	}
	return v
}

// TrafficAccount is the persisted day and month totals plus the pump total
// already folded in. Applied is monotonic for one process; a smaller total
// means the pump restarted and only the new bytes are added.
type TrafficAccount struct {
	DayKey     string
	MonthKey   string
	DayBytes   int64
	MonthBytes int64
	Applied    int64
}

// NoteTraffic folds a monotonic pump total into the local calendar day and month.
func NoteTraffic(prev TrafficAccount, total int64, now time.Time) TrafficAccount {
	day := now.Format("2006-01-02")
	month := now.Format("2006-01")
	if prev.DayKey != day {
		prev.DayBytes = 0
		prev.DayKey = day
	}
	if prev.MonthKey != month {
		prev.MonthBytes = 0
		prev.MonthKey = month
	}
	delta := total - prev.Applied
	if total < prev.Applied {
		delta = total
	}
	if delta < 0 {
		delta = 0
	}
	prev.DayBytes += delta
	prev.MonthBytes += delta
	prev.Applied = total
	return prev
}
