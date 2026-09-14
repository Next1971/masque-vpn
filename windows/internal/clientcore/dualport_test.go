package clientcore

import (
	"net"
	"testing"
)

func TestDualDialAddrs(t *testing.T) {
	primary := &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 4433}
	if got := dualDialAddrs(primary, 0); len(got) != 1 || got[0].Port != 4433 {
		t.Fatalf("no alt: %+v", got)
	}
	got := dualDialAddrs(primary, 2053)
	if len(got) != 2 || got[0].Port != 4433 || got[1].Port != 2053 {
		t.Fatalf("alt 2053: %+v", got)
	}
	if got := dualDialAddrs(primary, 4433); len(got) != 1 {
		t.Fatalf("same port should not race: %+v", got)
	}
}

func TestResolveDialAddrsLiteral(t *testing.T) {
	got, err := resolveDialAddrs("203.0.113.10:443", 0)
	if err != nil || len(got) != 1 || got[0].Port != 443 || got[0].IP.To4().String() != "203.0.113.10" {
		t.Fatalf("v4: %+v %v", got, err)
	}
	got, err = resolveDialAddrs("[2001:db8::1]:443", 2053)
	if err != nil || len(got) != 2 {
		t.Fatalf("v6+alt: %+v %v", got, err)
	}
	if got[0].IP.To16() == nil || got[0].IP.To4() != nil || got[0].Port != 443 || got[1].Port != 2053 {
		t.Fatalf("v6 ports: %+v", got)
	}
	if ServerHost("[2001:db8::1]:443") != "2001:db8::1" {
		t.Fatalf("ServerHost %q", ServerHost("[2001:db8::1]:443"))
	}
}

func TestListenUDPForFamily(t *testing.T) {
	c4, err := listenUDPFor(&net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	defer c4.Close()
	c6, err := listenUDPFor(&net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	defer c6.Close()
}

func TestValidateAltPort(t *testing.T) {
	if err := validateAltPort("h:4433", 0); err != nil {
		t.Fatal(err)
	}
	if err := validateAltPort("h:4433", 2053); err != nil {
		t.Fatal(err)
	}
	if err := validateAltPort("h:4433", 4433); err == nil {
		t.Fatal("same port")
	}
	if err := validateAltPort("h:4433", 70000); err == nil {
		t.Fatal("range")
	}
}
