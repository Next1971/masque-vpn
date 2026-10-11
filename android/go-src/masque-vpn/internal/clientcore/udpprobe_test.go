package clientcore

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestUDPReachableDoesNotLeakLoserProbes(t *testing.T) {
	reply, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer reply.Close()
	black, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer black.Close()

	base := runtime.NumGoroutine()
	echoDone := make(chan struct{})
	go func() {
		defer close(echoDone)
		buf := make([]byte, 2048)
		for {
			n, addr, err := reply.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = reply.WriteTo(buf[:n], addr)
		}
	}()

	rp := reply.LocalAddr().(*net.UDPAddr)
	bp := black.LocalAddr().(*net.UDPAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := udpReachable(ctx, &Profile{Server: rp.String(), AltPort: bp.Port})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected the answering address to win")
	}
	reply.Close()
	<-echoDone

	deadline := time.Now().Add(udpProbeTimeout + time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if left := runtime.NumGoroutine(); left > base {
		t.Fatalf("loser probe still running: before %d after %d", base, left)
	}
}
