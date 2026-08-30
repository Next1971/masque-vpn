package clientcore

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

const (
	reconnectMinBackoff = time.Second
	reconnectMaxBackoff = 15 * time.Second
)

// Pump keeps one TUN device and replaces the MASQUE session when QUIC dies.
// The TUN reader is started once: a second Session.Run on the same fd would
// steal packets. Used by the Android gomobile bridge.
type Pump struct {
	mu     sync.RWMutex
	sess   *Session
	dev    tun.Device
	tunErr error
}

func NewPump(sess *Session, dev tun.Device) *Pump {
	if sess != nil {
		sess.AttachTUN(dev)
	}
	return &Pump{sess: sess, dev: dev}
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

// RTT is the live session's smoothed QUIC RTT, or zero.
func (p *Pump) RTT() time.Duration {
	return p.session().RTT()
}

func (p *Pump) setSession(s *Session) {
	p.mu.Lock()
	p.sess = s
	p.mu.Unlock()
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

func (p *Pump) readConn(ctx context.Context, sess *Session) error {
	mtu, err := p.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = 1400
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
		inCount++
		if Verbose && inCount <= 6 {
			vlog("conn→TUN packet #%d: %s (%d bytes)", inCount, describePkt(buf[tunOffset:tunOffset+n]), n)
		}
		if _, err := p.dev.Write([][]byte{buf[:tunOffset+n]}, tunOffset); err != nil {
			p.setTunErr(err)
			return fmt.Errorf("tun write: %w", err)
		}
	}
}

func (p *Pump) readTUN(ctx context.Context) {
	mtu, err := p.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = 1400
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
			if _, err := sess.ipconn.WritePacket(pkt); err != nil {
				break
			}
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
