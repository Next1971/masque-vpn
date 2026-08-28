package clientcore

import (
	"net/netip"
	"testing"
)

func TestPrepareOutgoingDropsIPv6(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x60
	drop, rew := prepareOutgoing(pkt, netip.MustParseAddr("10.8.0.253"))
	if !drop || rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
}

func TestPrepareOutgoingRewritesLANSource(t *testing.T) {
	pkt := make([]byte, 28)
	pkt[0] = 0x45
	pkt[2], pkt[3] = 0, 28
	pkt[8] = 64
	pkt[9] = 17 // UDP
	pkt[12], pkt[13], pkt[14], pkt[15] = 192, 168, 0, 14
	pkt[16], pkt[17], pkt[18], pkt[19] = 1, 1, 1, 1
	pkt[20], pkt[21] = 0xC0, 0x00
	pkt[22], pkt[23] = 0, 53
	pkt[24], pkt[25] = 0, 8
	assigned := netip.MustParseAddr("10.8.0.254")
	drop, rew := prepareOutgoing(pkt, assigned)
	if drop || !rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
	src := netip.AddrFrom4([4]byte{pkt[12], pkt[13], pkt[14], pkt[15]})
	if src != assigned {
		t.Fatalf("src=%s want %s", src, assigned)
	}
}

func TestPrepareOutgoingKeepsAssignedSource(t *testing.T) {
	pkt := make([]byte, 20)
	pkt[0] = 0x45
	pkt[8] = 64
	pkt[12], pkt[13], pkt[14], pkt[15] = 10, 8, 0, 253
	assigned := netip.MustParseAddr("10.8.0.253")
	drop, rew := prepareOutgoing(pkt, assigned)
	if drop || rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
}
