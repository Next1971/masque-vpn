package clientcore

import (
	"testing"
	"time"
)

func TestTransportMode(t *testing.T) {
	if TransportMode("") != "auto" || TransportMode("AUTO") != "auto" || TransportMode("quic") != "auto" {
		t.Fatal(TransportMode(""), TransportMode("AUTO"), TransportMode("quic"))
	}
	if TransportMode(" UDP ") != "udp" || TransportMode("tcp") != "tcp" {
		t.Fatal(TransportMode(" UDP "), TransportMode("tcp"))
	}
}

func TestClampMTU(t *testing.T) {
	if ClampMTU(0) != TunnelMTU || ClampMTU(100) != TunnelMTU || ClampMTU(9000) != TunnelMTU {
		t.Fatal("out of range")
	}
	if ClampMTU(1280) != 1280 || ClampMTU(1340) != 1340 || ClampMTU(1500) != 1500 {
		t.Fatal("in range")
	}
}

func TestValidateKeepsChosenMTU(t *testing.T) {
	p := &Profile{Server: "example.com:443", ServerName: "example.com", MTU: 1400}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.MTU != 1400 {
		t.Fatalf("mtu %d", p.MTU)
	}
	p.MTU = 100
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.MTU != TunnelMTU {
		t.Fatalf("mtu %d", p.MTU)
	}
}

func TestNoteTrafficDayMonthAndRestart(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	a := NoteTraffic(TrafficAccount{}, 1000, now)
	if a.DayBytes != 1000 || a.MonthBytes != 1000 || a.Applied != 1000 || a.DayKey != "2026-10-11" {
		t.Fatal(a)
	}
	b := NoteTraffic(a, 1500, now)
	if b.DayBytes != 1500 || b.MonthBytes != 1500 || b.Applied != 1500 {
		t.Fatal(b)
	}
	c := NoteTraffic(b, 200, now)
	if c.DayBytes != 1700 || c.MonthBytes != 1700 || c.Applied != 200 {
		t.Fatal(c)
	}
	next := now.Add(24 * time.Hour)
	d := NoteTraffic(c, 250, next)
	if d.DayKey != "2026-10-12" || d.DayBytes != 50 || d.MonthBytes != 1750 || d.Applied != 250 {
		t.Fatal(d)
	}
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	e := NoteTraffic(d, 300, nov)
	if e.MonthKey != "2026-11" || e.MonthBytes != 50 || e.DayBytes != 50 || e.Applied != 300 {
		t.Fatal(e)
	}
}
