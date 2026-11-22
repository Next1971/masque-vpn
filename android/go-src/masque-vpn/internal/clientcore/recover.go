package clientcore

import "time"

const (
	// tunnelProbeEvery is how often an idle tunnel is asked to carry one
	// ICMP echo. Two windows with no IP packet at all mean the session is
	// up but no longer moves data.
	tunnelProbeEvery = 10 * time.Second
	// tunnelProbeMissLimit is consecutive probe windows with no tunnel IP.
	tunnelProbeMissLimit = 2
)

// dataWatch counts probe windows in which nothing came back through the
// tunnel. Any inbound IP packet, including the echo reply, clears it.
type dataWatch struct {
	misses int
}

// observe records one probe window. arrived is true when any tunnel IP
// packet was received since the previous probe. The bool is true when the
// data plane should be treated as down.
func (w *dataWatch) observe(arrived bool) bool {
	if arrived {
		w.misses = 0
		return false
	}
	w.misses++
	return w.misses >= tunnelProbeMissLimit
}

// RecoverPlan is the mid-session transport ladder. It does not guess why
// the path stopped carrying data. Automatic mode dials UDP once and, if
// that session also fails the data check, dials TCP and stays there.
// UDP-only never opens TCP. TCP-only stays on TCP. A failed ladder does
// not stick: the next attempt starts at UDP again.
type RecoverPlan struct {
	mode      string
	stickyTCP bool
}

func NewRecoverPlan(transport string) RecoverPlan {
	return RecoverPlan{mode: TransportMode(transport)}
}

// Steps is the ordered list of transports to try after the live session
// stopped carrying data.
func (p RecoverPlan) Steps() []string {
	switch p.mode {
	case "tcp":
		return []string{"tcp"}
	case "udp":
		return []string{"udp"}
	default:
		if p.stickyTCP {
			return []string{"tcp"}
		}
		return []string{"udp", "tcp"}
	}
}

// ConfirmTCP sticks to TCP after automatic mode has seen TCP actually
// carry data. A later failure redials TCP instead of returning to UDP.
func (p *RecoverPlan) ConfirmTCP(step string) {
	if p.mode == "auto" && step == "tcp" {
		p.stickyTCP = true
	}
}
