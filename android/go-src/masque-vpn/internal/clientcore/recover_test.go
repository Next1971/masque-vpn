package clientcore

import "testing"

func TestDataWatchTwoMisses(t *testing.T) {
	var w dataWatch
	if w.observe(false) {
		t.Fatal("first silent window is not yet a failure")
	}
	if !w.observe(false) {
		t.Fatal("second silent window should restore")
	}
}

func TestDataWatchResetsOnArrival(t *testing.T) {
	var w dataWatch
	if w.observe(false) {
		t.Fatal("first miss")
	}
	if w.observe(true) || w.misses != 0 {
		t.Fatal("an inbound packet should clear the miss count")
	}
	if w.observe(false) {
		t.Fatal("one miss after a reset is not a failure")
	}
}

func TestRecoverPlanAutomaticLadder(t *testing.T) {
	p := NewRecoverPlan("auto")
	got := p.Steps()
	if len(got) != 2 || got[0] != "udp" || got[1] != "tcp" {
		t.Fatalf("auto steps: %v", got)
	}
	p.ConfirmTCP("udp")
	got = p.Steps()
	if len(got) != 2 || got[0] != "udp" || got[1] != "tcp" {
		t.Fatalf("udp success must not stick: %v", got)
	}
	p.ConfirmTCP("tcp")
	got = p.Steps()
	if len(got) != 1 || got[0] != "tcp" {
		t.Fatalf("tcp should stick: %v", got)
	}
}

func TestRecoverPlanForcedMode(t *testing.T) {
	udp := NewRecoverPlan("udp")
	udp.ConfirmTCP("tcp")
	if got := udp.Steps(); len(got) != 1 || got[0] != "udp" {
		t.Fatalf("udp only: %v", got)
	}
	tcp := NewRecoverPlan("tcp")
	if got := tcp.Steps(); len(got) != 1 || got[0] != "tcp" {
		t.Fatalf("tcp only: %v", got)
	}
}

func TestRecoverPlanRetriesLadder(t *testing.T) {
	p := NewRecoverPlan("")
	_ = p.Steps()
	got := p.Steps()
	if len(got) != 2 || got[0] != "udp" || got[1] != "tcp" {
		t.Fatalf("a failed ladder should start at UDP again: %v", got)
	}
}
