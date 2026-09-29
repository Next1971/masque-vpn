package clientcore

import (
	"net/netip"
	"testing"

	connectip "github.com/quic-go/connect-ip-go"
)

func TestPrefixesFromAssignmentSkipsRejected(t *testing.T) {
	ok := netip.MustParsePrefix("10.8.0.253/32")
	got := prefixesFromAssignment([]connectip.AssignedAddress{
		{IPPrefix: ok},
		{IPPrefix: netip.PrefixFrom(netip.IPv4Unspecified(), 32)},
	})
	if len(got) != 1 || got[0] != ok {
		t.Fatalf("got %v", got)
	}
}

func TestTunnelGateway4(t *testing.T) {
	gw := tunnelGateway4(netip.MustParseAddr("10.8.0.253"))
	if gw != netip.MustParseAddr("10.8.0.1") {
		t.Fatalf("gateway %s", gw)
	}
}

func TestICMPEchoReplyFrom(t *testing.T) {
	src := netip.MustParseAddr("10.8.0.253")
	gw := netip.MustParseAddr("10.8.0.1")
	req := icmpEchoRequest(gw, src, 1)
	// Turn the request into a reply from the gateway.
	req[12], req[13], req[14], req[15] = 10, 8, 0, 1
	req[16], req[17], req[18], req[19] = 10, 8, 0, 253
	ihl := int(req[0]&0x0f) * 4
	req[ihl] = 0
	if !icmpEchoReplyFrom(req, gw) {
		t.Fatal("expected echo reply")
	}
	if icmpEchoReplyFrom(req, src) {
		t.Fatal("reply source is not the client")
	}
}
