package clientcore

import (
	"fmt"
	"net/netip"
)

// Processes outgoing packet IP headers before proxying.
//
// Problem: some operating systems (notably Windows with certain routing
// into TUN) generate packets with TTL=1 (IPv4) / Hop Limit=1 (IPv6). The
// connect-ip-go library decrements TTL while proxying IP under RFC 9484 and, if
// it becomes 0, MUST drop the packet ("datagram Hop Limit too small: 1").
// This drops all client traffic before it is sent to the server.
//
// Solution: before sending, raise an excessively low TTL to a safe
// value (minTTL→64) and recompute the IPv4 header checksum.
// This is done in the client core, so the fix is shared by all platforms
// (Linux/Windows/Android). For valid packets (with a normal TTL), the function
// makes no changes.

const (
	// minTTL: raise packet TTL/Hop Limit to fixTTL if it is lower than this.
	// Threshold 2, because connect-ip decrements it and requires a result ≥ 1.
	minTTL = 2
	// fixTTL: value to which an excessively low TTL is raised.
	fixTTL = 64
)

// normalizeTTL checks the packet IP version and, if TTL/Hop Limit < minTTL,
// raises it to fixTTL. For IPv4, it recomputes the header checksum.
// It returns the original TTL (for diagnostics) and whether the packet was modified.
// pkt is the full IP packet (starting with version/IHL).
func normalizeTTL(pkt []byte) (origTTL int, fixed bool) {
	if len(pkt) < 1 {
		return -1, false
	}
	version := pkt[0] >> 4
	switch version {
	case 4:
		// IPv4: minimum header is 20 bytes. TTL is byte 8; checksum is bytes 10-11.
		if len(pkt) < 20 {
			return -1, false
		}
		origTTL = int(pkt[8])
		if origTTL >= minTTL {
			return origTTL, false
		}
		pkt[8] = fixTTL
		// Recompute the header checksum (by IHL).
		ihl := int(pkt[0]&0x0f) * 4
		if ihl < 20 || ihl > len(pkt) {
			ihl = 20
		}
		pkt[10] = 0
		pkt[11] = 0
		csum := ipv4Checksum(pkt[:ihl])
		pkt[10] = byte(csum >> 8)
		pkt[11] = byte(csum & 0xff)
		return origTTL, true
	case 6:
		// IPv6: fixed header is 40 bytes. Hop Limit is byte 7.
		// There is no checksum in the IPv6 header.
		if len(pkt) < 40 {
			return -1, false
		}
		origTTL = int(pkt[7])
		if origTTL >= minTTL {
			return origTTL, false
		}
		pkt[7] = fixTTL
		return origTTL, true
	default:
		return -1, false
	}
}

// ipv4Checksum calculates the IPv4 header checksum (RFC 791):
// one’s complement of the sum of 16-bit words.
func ipv4Checksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(hdr); i += 2 {
		sum += uint32(hdr[i])<<8 | uint32(hdr[i+1])
	}
	if len(hdr)%2 == 1 {
		sum += uint32(hdr[len(hdr)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// describePkt returns a concise human-readable IP packet description for logs:
// version, src→dst, protocol, and TTL/Hop Limit. Used to diagnose the
// incoming conn→TUN path.
func describePkt(pkt []byte) string {
	if len(pkt) < 1 {
		return "empty"
	}
	switch pkt[0] >> 4 {
	case 4:
		if len(pkt) < 20 {
			return "short-ipv4"
		}
		src := netip.AddrFrom4([4]byte{pkt[12], pkt[13], pkt[14], pkt[15]})
		dst := netip.AddrFrom4([4]byte{pkt[16], pkt[17], pkt[18], pkt[19]})
		return fmt.Sprintf("IPv4 %s→%s proto=%d ttl=%d", src, dst, pkt[9], pkt[8])
	case 6:
		if len(pkt) < 40 {
			return "short-ipv6"
		}
		var s, d [16]byte
		copy(s[:], pkt[8:24])
		copy(d[:], pkt[24:40])
		return fmt.Sprintf("IPv6 %s→%s next=%d hlim=%d",
			netip.AddrFrom16(s), netip.AddrFrom16(d), pkt[6], pkt[7])
	default:
		return fmt.Sprintf("unknown-version %d", pkt[0]>>4)
	}
}

// prepareOutgoing drops packets the CONNECT-IP server would reject, and rewrites
// a wrong IPv4 source to the assigned tunnel address (some Android OEMs source
// from Wi-Fi when the TUN is configured as /32).
//
// Returns drop=true to skip WritePacket; rewritten=true if the IPv4 header changed.
func prepareOutgoing(pkt []byte, assigned netip.Addr) (drop, rewritten bool) {
	if len(pkt) < 1 || !assigned.IsValid() {
		return true, false
	}
	switch pkt[0] >> 4 {
	case 6:
		// Tunnel is IPv4-only today; sink IPv6 on the Android TUN and drop here
		// so CONNECT-IP never sees :: / fe80 sources.
		return true, false
	case 4:
		if len(pkt) < 20 || !assigned.Is4() {
			return true, false
		}
		src := netip.AddrFrom4([4]byte{pkt[12], pkt[13], pkt[14], pkt[15]})
		if src == assigned {
			return false, false
		}
		b := assigned.As4()
		pkt[12], pkt[13], pkt[14], pkt[15] = b[0], b[1], b[2], b[3]
		ihl := int(pkt[0]&0x0f) * 4
		if ihl < 20 || ihl > len(pkt) {
			ihl = 20
		}
		pkt[10] = 0
		pkt[11] = 0
		csum := ipv4Checksum(pkt[:ihl])
		pkt[10] = byte(csum >> 8)
		pkt[11] = byte(csum & 0xff)
		return false, true
	default:
		return true, false
	}
}
