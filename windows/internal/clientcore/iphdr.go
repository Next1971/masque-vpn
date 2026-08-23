package clientcore

import (
	"fmt"
	"net/netip"
)

// Processing the IP header of outgoing packets before proxying.
//
// Problem: some operating systems (in particular Windows with certain routing
// setups in TUN) generate packets with TTL=1 (IPv4) / Hop Limit=1 (IPv6).
// The connect-ip-go library, according to RFC 9484, decrements TTL when
// proxying IP packets and, if it becomes 0, MUST drop the packet
// ("datagram Hop Limit too small: 1").
// As a result, all client traffic gets dropped before it even reaches the server.
//
// Solution: before sending, raise too-small TTL to a safe value (minTTL→64)
// and recalculate the IPv4 header checksum. This is done in the client core,
// so the fix applies to all platforms (Linux/Windows/Android).
// On valid packets (TTL already normal), the function does nothing.

const (
	// minTTL — if a packet's TTL/Hop Limit is below this value, raise it to fixTTL.
	// Threshold 2, because connect-ip decrements and requires the result to be ≥ 1.
	minTTL = 2
	// fixTTL — value to which too-small TTL is raised.
	fixTTL = 64
)

// normalizeTTL checks the IP version and, if TTL/Hop Limit < minTTL,
// raises it to fixTTL. For IPv4 it recalculates the header checksum.
// Returns the original TTL (for diagnostics) and whether the packet was modified.
// pkt is a full IP packet (starting with version/IHL).
func normalizeTTL(pkt []byte) (origTTL int, fixed bool) {
	if len(pkt) < 1 {
		return -1, false
	}
	version := pkt[0] >> 4
	switch version {
	case 4:
		// IPv4: minimum header is 20 bytes. TTL is byte 8. Checksum is bytes 10-11.
		if len(pkt) < 20 {
			return -1, false
		}
		origTTL = int(pkt[8])
		if origTTL >= minTTL {
			return origTTL, false
		}
		pkt[8] = fixTTL
		// Recalculate header checksum (by IHL).
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
		// IPv6: fixed 40-byte header. Hop Limit is byte 7.
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

// ipv4Checksum computes the IPv4 header checksum (RFC 791):
// one's complement sum of 16-bit words.
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

// describePkt returns a short human-readable description of an IP packet for logs:
// version, src→dst, protocol, and TTL/Hop Limit. Used for diagnostics on the
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
