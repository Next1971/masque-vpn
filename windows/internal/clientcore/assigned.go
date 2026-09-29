package clientcore

import (
	"net/netip"

	connectip "github.com/quic-go/connect-ip-go"
)

// SplitAssigned picks the first IPv4 and first native IPv6 prefix from the
// CONNECT-IP assignment. IPv4-mapped IPv6 addresses are ignored for v6.
func SplitAssigned(prefixes []netip.Prefix) (v4, v6 netip.Prefix) {
	for _, p := range prefixes {
		a := p.Addr()
		if a.Is4() && !v4.IsValid() {
			v4 = p
			continue
		}
		if a.Is6() && !a.Is4In6() && !v6.IsValid() {
			v6 = p
		}
	}
	return v4, v6
}

// prefixesFromAssignment keeps accepted ADDRESS_ASSIGN prefixes.
// connect-ip-go 0.4 replaced LocalPrefixes with ReceiveAddressAssignment.
// The client waits for the server's unsolicited assignment and does not send
// ADDRESS_REQUEST, so a 0.3 server still accepts the session.
func prefixesFromAssignment(assigned []connectip.AssignedAddress) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(assigned))
	for _, a := range assigned {
		if a.Rejected() || !a.IPPrefix.IsValid() {
			continue
		}
		out = append(out, a.IPPrefix)
	}
	return out
}
