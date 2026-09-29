package clientcore

import "net/netip"

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
