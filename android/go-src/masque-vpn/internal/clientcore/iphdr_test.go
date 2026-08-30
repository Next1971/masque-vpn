package clientcore

import (
	"net/netip"
	"testing"
)

func TestSplitAssigned(t *testing.T) {
	v4, v6 := SplitAssigned([]netip.Prefix{
		netip.MustParsePrefix("10.8.0.2/32"),
		netip.MustParsePrefix("fd00:8::2/128"),
	})
	if v4.String() != "10.8.0.2/32" || v6.String() != "fd00:8::2/128" {
		t.Fatalf("v4=%s v6=%s", v4, v6)
	}
	v4only, v6none := SplitAssigned([]netip.Prefix{netip.MustParsePrefix("10.8.0.3/32")})
	if !v4only.IsValid() || v6none.IsValid() {
		t.Fatalf("v4-only: v4=%v v6=%v", v4only, v6none)
	}
}

func TestPrepareOutgoingDropsIPv6WithoutAssignment(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x60
	drop, rew := prepareOutgoing(pkt, []netip.Prefix{netip.MustParsePrefix("10.8.0.253/32")})
	if !drop || rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
}

func TestPrepareOutgoingKeepsAssignedIPv6(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x60
	v6 := netip.MustParseAddr("fd00:8::2")
	b := v6.As16()
	copy(pkt[8:24], b[:])
	drop, rew := prepareOutgoing(pkt, []netip.Prefix{
		netip.MustParsePrefix("10.8.0.253/32"),
		netip.MustParsePrefix("fd00:8::2/128"),
	})
	if drop || rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
}

func TestPrepareOutgoingRewritesIPv6Source(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x60
	ll := netip.MustParseAddr("fe80::1")
	llb := ll.As16()
	copy(pkt[8:24], llb[:])
	assigned := netip.MustParsePrefix("fd00:8::2/128")
	drop, rew := prepareOutgoing(pkt, []netip.Prefix{netip.MustParsePrefix("10.8.0.1/32"), assigned})
	if drop || !rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
	var src16 [16]byte
	copy(src16[:], pkt[8:24])
	if netip.AddrFrom16(src16) != assigned.Addr() {
		t.Fatalf("src=%s", netip.AddrFrom16(src16))
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
	drop, rew := prepareOutgoing(pkt, []netip.Prefix{netip.PrefixFrom(assigned, 32)})
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
	drop, rew := prepareOutgoing(pkt, []netip.Prefix{netip.PrefixFrom(assigned, 32)})
	if drop || rew {
		t.Fatalf("drop=%v rewritten=%v", drop, rew)
	}
}
