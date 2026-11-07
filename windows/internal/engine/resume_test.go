//go:build windows

package engine

import (
	"context"
	"testing"
	"time"
)

func TestWallClockJumped(t *testing.T) {
	base := time.Now().UnixNano()
	if wallClockJumped(base, base+int64(19*time.Second)) {
		t.Fatal("19s gap should stay under the sleep threshold")
	}
	if !wallClockJumped(base, base+int64(2*time.Hour)) {
		t.Fatal("multi-hour gap should count as sleep")
	}
	if wallClockJumped(base, base) {
		t.Fatal("equal clocks are not a jump")
	}
	if wallClockJumped(base, base-int64(time.Hour)) {
		t.Fatal("a backward step is not a jump")
	}
}

func TestNotifyResumeIgnoredWhenIdle(t *testing.T) {
	e := New(nil)
	e.NotifyResume("test")
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resumeReq {
		t.Fatal("resume while disconnected should be ignored")
	}
}

func TestNotifyResumeSignalsAndDebounces(t *testing.T) {
	e := New(nil)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.mu.Lock()
	e.cancel = cancel
	runCtx, runCancel := context.WithCancel(context.Background())
	e.runCancel = runCancel
	e.mu.Unlock()

	e.NotifyResume("power resume")
	if runCtx.Err() == nil {
		t.Fatal("resume should cancel the running attempt")
	}
	e.mu.Lock()
	if !e.resumeReq {
		e.mu.Unlock()
		t.Fatal("resume should request a rebuild")
	}
	e.resumeReq = false
	e.mu.Unlock()

	e.NotifyResume("power resume again")
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resumeReq {
		t.Fatal("a second resume inside the debounce window should be ignored")
	}
}
