package clientcore

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"sync/atomic"
	"time"
)

const udpProbeTimeout = 3 * time.Second

// udpReachable reports whether any profile UDP address answers a QUIC
// version-negotiation probe. Silence means UDP is unusable and Connect
// should open HTTP/2 instead. A live server answers in one round trip.
func udpReachable(ctx context.Context, p *Profile) (bool, error) {
	addrs, err := resolveDialAddrs(p.Server, p.AltPort)
	if err != nil {
		return false, err
	}
	pctx, cancel := context.WithTimeout(ctx, udpProbeTimeout)
	defer cancel()

	var found atomic.Bool
	var pending int
	done := make(chan struct{})
	for _, addr := range addrs {
		pending++
		go func(a *net.UDPAddr) {
			defer func() { done <- struct{}{} }()
			ok, err := probeUDP(pctx, a)
			if err != nil || !ok {
				return
			}
			found.Store(true)
		}(addr)
	}
	finished := 0
	for finished < pending {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-done:
			finished++
			if found.Load() {
				return true, nil
			}
		}
	}
	return false, nil
}

// probeUDP sends one QUIC long-header datagram with an unknown version.
// Any UDP reply from that address is enough; CONNECT-IP is not opened.
func probeUDP(ctx context.Context, raddr *net.UDPAddr) (bool, error) {
	network := "udp4"
	if raddr.IP.To4() == nil {
		network = "udp6"
	}
	pc, err := net.ListenPacket(network, "")
	if err != nil {
		return false, err
	}
	defer pc.Close()

	pkt := make([]byte, 1200)
	pkt[0] = 0xc0
	pkt[1], pkt[2], pkt[3], pkt[4] = 0x0a, 0x0a, 0x0a, 0x0a
	pkt[5] = 8
	if _, err := rand.Read(pkt[6:14]); err != nil {
		return false, err
	}
	pkt[14] = 8
	if _, err := rand.Read(pkt[15:23]); err != nil {
		return false, err
	}
	if _, err := pc.WriteTo(pkt, raddr); err != nil {
		return false, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = pc.SetReadDeadline(dl)
	}
	buf := make([]byte, 2048)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return false, ctx.Err()
			}
			return false, nil
		}
		ua, ok := from.(*net.UDPAddr)
		if !ok || ua == nil || !ua.IP.Equal(raddr.IP) || ua.Port != raddr.Port || n == 0 {
			continue
		}
		return true, nil
	}
}
