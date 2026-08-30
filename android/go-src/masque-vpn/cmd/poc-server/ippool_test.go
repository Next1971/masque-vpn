package main

import (
	"net/netip"
	"testing"
)

func TestIPv4PoolSticky(t *testing.T) {
	p, err := NewIPPool("10.8.0.0/24", netip.MustParseAddr("10.8.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	a, lease, err := p.AllocateFor("cn-a")
	if err != nil {
		t.Fatal(err)
	}
	p.Release("cn-a", lease, a)
	b, _, err := p.AllocateFor("cn-a")
	if err != nil {
		t.Fatal(err)
	}
	if a.Addr() != b.Addr() {
		t.Fatalf("sticky: %s vs %s", a, b)
	}
}

func TestIPv6PoolAssignsHost128(t *testing.T) {
	p, err := NewIPPool("fd00:8::/64", netip.MustParseAddr("fd00:8::1"))
	if err != nil {
		t.Fatal(err)
	}
	a, lease, err := p.AllocateFor("phone")
	if err != nil {
		t.Fatal(err)
	}
	if a.Bits() != 128 || a.Addr() == netip.MustParseAddr("fd00:8::1") {
		t.Fatalf("got %s", a)
	}
	p.Release("phone", lease, a)
	b, _, err := p.AllocateFor("phone")
	if err != nil {
		t.Fatal(err)
	}
	if a.Addr() != b.Addr() {
		t.Fatalf("sticky v6: %s vs %s", a, b)
	}
}

func TestIPv6PoolDistinctClients(t *testing.T) {
	p, err := NewIPPool("fd00:8::/64", netip.MustParseAddr("fd00:8::1"))
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := p.AllocateFor("c1")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := p.AllocateFor("c2")
	if err != nil {
		t.Fatal(err)
	}
	if a.Addr() == b.Addr() {
		t.Fatalf("same address %s", a)
	}
}
