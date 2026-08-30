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
