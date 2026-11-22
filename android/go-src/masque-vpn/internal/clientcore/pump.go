package clientcore

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

const (
	reconnectMinBackoff = time.Second
	reconnectMaxBackoff = 15 * time.Second
	// QUIC keepalives are every 15s. Three missed receive windows means the
	// path is blackholed and ReadPacket may stay blocked until idle timeout.
	pathSilenceStep  = 15 * time.Second
	pathSilenceLimit = 45 * time.Second
)

// Pump keeps one TUN device and replaces the MASQUE session when QUIC dies.
// The TUN reader is started once: a second Session.Run on the same fd would
// steal packets. Used by the Android gomobile bridge.
type Pump struct {
	mu          sync.RWMutex
	wmu         sync.Mutex
	sess        *Session
	dev         tun.Device
	tunErr      error
	bytes       atomic.Int64
	inbound     atomic.Bool
	dataWatchOn atomic.Bool
}

func NewPump(sess *Session, dev tun.Device) *Pump {
	if sess != nil {
		sess.AttachTUN(dev)
	}
	return &Pump{sess: sess, dev: dev}
}

// EnableDataWatch turns on the ICMP data-plane check. Android can replace
// the session when it fails. iOS cannot open a new socket from here, so it
// leaves the check off and keeps the QUIC silence watch only.
func (p *Pump) EnableDataWatch() {
	if p != nil {
		p.dataWatchOn.Store(true)
	}
}

// Bytes is tunnel IP traffic since this pump started, both directions.
func (p *Pump) Bytes() int64 {
	if p == nil {
		return 0
	}
	return p.bytes.Load()
}

func (p *Pump) addBytes(n int) {
	if n > 0 {
		p.bytes.Add(int64(n))
	}
}

// Run forwards until ctx is cancelled or the TUN device fails.
// redial must Dial a new CONNECT-IP session without a TUN (dev=nil).
func (p *Pump) Run(ctx context.Context, redial func(context.Context) (*Session, error)) error {
	if p.dev == nil {
		return fmt.Errorf("no TUN device")
	}

	go p.readTUN(ctx)

	backoff := reconnectMinBackoff
	for {
		if err := ctx.Err(); err != nil {
			p.closeSession()
			return err
		}
		if err := p.tunError(); err != nil {
			p.closeSession()
			return err
		}

		sess := p.session()
		if sess == nil {
			s, err := redial(ctx)
			if err != nil {
				if ctx.Err() != nil {
					p.closeSession()
					return ctx.Err()
				}
				log.Printf("reconnect failed: %v (retry in %s)", err, backoff)
				if !sleepCtx(ctx, backoff) {
					p.closeSession()
					return ctx.Err()
				}
				backoff *= 2
				if backoff > reconnectMaxBackoff {
					backoff = reconnectMaxBackoff
				}
				continue
			}
			if err := p.tunError(); err != nil {
				s.Close()
				p.closeSession()
				return err
			}
			s.AttachTUN(p.dev)
			p.setSession(s)
			backoff = reconnectMinBackoff
			sess = s
		}

		err := p.readConn(ctx, sess)
		p.closeSession()

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if tErr := p.tunError(); tErr != nil {
			return tErr
		}
		log.Printf("session ended, reconnecting: %v", err)
	}
}

func (p *Pump) session() *Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sess
}

// RTT is the live session's smoothed path RTT, or zero.
func (p *Pump) RTT() time.Duration {
	return p.session().RTT()
}

func (p *Pump) setSession(s *Session) {
	p.mu.Lock()
	p.sess = s
	p.mu.Unlock()
}

// Close tears down the live session so a blocked ReadPacket returns.
// Disconnect must cancel the Run context first, then call Close; otherwise
// Run treats the drop as a network failure and reconnects. Safe from another
// goroutine; nil-safe.
func (p *Pump) Close() {
	if p == nil {
		return
	}
	p.closeSession()
}

func (p *Pump) closeSession() {
	p.mu.Lock()
	s := p.sess
	p.sess = nil
	p.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

func (p *Pump) tunError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tunErr
}

func (p *Pump) setTunErr(err error) {
	p.mu.Lock()
	if p.tunErr == nil {
		p.tunErr = err
	}
	p.mu.Unlock()
}

// pathQuiet advances the silence timer when no new QUIC packet arrived.
// A moving counter resets it. drop is true once quiet reaches limit.
func pathQuiet(prev, now uint64, quiet, step, limit time.Duration) (uint64, time.Duration, bool) {
	if now != prev {
		return now, 0, false
	}
	quiet += step
	return prev, quiet, quiet >= limit
}

func (p *Pump) watchPath(ctx context.Context, sess *Session) {
	if sess == nil || sess.qconn == nil {
		return
	}
	tick := time.NewTicker(pathSilenceStep)
	defer tick.Stop()
	last := sess.qconn.ConnectionStats().PacketsReceived
	quiet := time.Duration(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.done:
			return
		case <-tick.C:
			now := sess.qconn.ConnectionStats().PacketsReceived
			var drop bool
			last, quiet, drop = pathQuiet(last, now, quiet, pathSilenceStep, pathSilenceLimit)
			if !drop {
				continue
			}
			log.Printf("no QUIC packets for %s; reconnecting", quiet)
			sess.Abandon("path silent")
			return
		}
	}
}

func (p *Pump) markInbound() { p.inbound.Store(true) }

func (p *Pump) takeInbound() bool { return p.inbound.Swap(false) }

func (p *Pump) writePacket(sess *Session, pkt []byte) error {
	if sess == nil || sess.ipconn == nil {
		return fmt.Errorf("no session")
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, err := sess.ipconn.WritePacket(pkt)
	return err
}

// watchData sends an ICMP echo through the live session every probe window.
// QUIC keepalives can still arrive after the CONNECT-IP data plane has
// stopped, so packet counters are not the signal. Two windows with no
// tunnel IP at all abandon the session; the caller then runs the ladder.
func (p *Pump) watchData(ctx context.Context, sess *Session) {
	if sess == nil || sess.ipconn == nil {
		return
	}
	v4, _ := SplitAssigned(sess.AssignedPrefixes)
	if !v4.IsValid() {
		return
	}
	gw := tunnelGateway4(v4.Addr())
	tick := time.NewTicker(tunnelProbeEvery)
	defer tick.Stop()
	var watch dataWatch
	var sent bool
	var seq uint16
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.done:
			return
		case <-tick.C:
			if sent && watch.observe(p.takeInbound()) {
				log.Printf("tunnel data probe missed %d times; restoring", tunnelProbeMissLimit)
				sess.Abandon("tunnel data stopped")
				return
			}
			sent = true
			seq++
			if err := p.writePacket(sess, icmpEchoRequest(v4.Addr(), gw, seq)); err != nil {
				return
			}
		}
	}
}

func (p *Pump) readConn(ctx context.Context, sess *Session) error {
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	p.inbound.Store(false)
	go p.watchPath(watchCtx, sess)
	if p.dataWatchOn.Load() {
		go p.watchData(watchCtx, sess)
	}

	gw := netip.Addr{}
	if v4, _ := SplitAssigned(sess.AssignedPrefixes); v4.IsValid() {
		gw = tunnelGateway4(v4.Addr())
	}

	mtu, err := p.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = TunnelMTU
	}
	buf := make([]byte, tunOffset+mtu+64)
	var inCount int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := sess.ipconn.ReadPacket(buf[tunOffset:])
		if err != nil {
			return fmt.Errorf("conn read: %w", err)
		}
		pkt := buf[tunOffset : tunOffset+n]
		p.markInbound()
		if gw.IsValid() && ourEchoReply(pkt, gw) {
			continue
		}
		inCount++
		if Verbose && inCount <= 6 {
			vlog("conn→TUN packet #%d: %s (%d bytes)", inCount, describePkt(pkt), n)
		}
		if _, err := p.dev.Write([][]byte{buf[:tunOffset+n]}, tunOffset); err != nil {
			p.setTunErr(err)
			return fmt.Errorf("tun write: %w", err)
		}
		p.addBytes(n)
	}
}

func (p *Pump) readTUN(ctx context.Context) {
	mtu, err := p.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = TunnelMTU
	}
	batch := p.dev.BatchSize()
	if batch < 1 {
		batch = 1
	}
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunOffset+mtu+64)
	}
	var fixedCount int
	for {
		k, err := p.dev.Read(bufs, sizes, tunOffset)
		if err != nil {
			p.setTunErr(fmt.Errorf("tun read: %w", err))
			p.closeSession()
			return
		}
		if ctx.Err() != nil {
			return
		}
		sess := p.session()
		if sess == nil || sess.ipconn == nil {
			continue
		}
		for i := 0; i < k; i++ {
			pkt := bufs[i][tunOffset : tunOffset+sizes[i]]
			if orig, fixed := normalizeTTL(pkt); fixed {
				fixedCount++
				if fixedCount <= 3 {
					vlog("raised low TTL/HopLimit %d→%d on outgoing packet (%d bytes)", orig, fixTTL, len(pkt))
				}
			}
			if drop, _ := prepareOutgoing(pkt, sess.AssignedPrefixes); drop {
				continue
			}
			if err := p.writePacket(sess, pkt); err != nil {
				break
			}
			p.addBytes(len(pkt))
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
