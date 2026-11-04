package clientcore

import "testing"

func TestPathQuietResetsWhenPacketsArrive(t *testing.T) {
	prev, quiet, drop := pathQuiet(10, 10, 0, pathSilenceStep, pathSilenceLimit)
	if drop || quiet != pathSilenceStep || prev != 10 {
		t.Fatalf("silent step: prev=%d quiet=%s drop=%v", prev, quiet, drop)
	}
	prev, quiet, drop = pathQuiet(prev, 11, quiet, pathSilenceStep, pathSilenceLimit)
	if drop || quiet != 0 || prev != 11 {
		t.Fatalf("reset: prev=%d quiet=%s drop=%v", prev, quiet, drop)
	}
}

func TestPathQuietDropsAtLimit(t *testing.T) {
	prev, quiet, drop := pathQuiet(4, 4, pathSilenceLimit-pathSilenceStep, pathSilenceStep, pathSilenceLimit)
	if !drop || quiet != pathSilenceLimit || prev != 4 {
		t.Fatalf("drop: prev=%d quiet=%s drop=%v", prev, quiet, drop)
	}
}
