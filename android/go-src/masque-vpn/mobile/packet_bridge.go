package mobile

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

const bridgeQueue = 64

// bridgeTUN is a userspace tun.Device. iOS has no TUN fd in a Packet
// Tunnel; Swift copies packets between NEPacketTunnelFlow and this queue.
type bridgeTUN struct {
	mtu       int
	fromOS    chan *[]byte
	toOS      chan *[]byte
	closed    chan struct{}
	closeOnce sync.Once
	events    chan tun.Event
}

func newBridgeTUN(mtu int) *bridgeTUN {
	if mtu <= 0 {
		mtu = 1400
	}
	t := &bridgeTUN{
		mtu:    mtu,
		fromOS: make(chan *[]byte, bridgeQueue),
		toOS:   make(chan *[]byte, bridgeQueue),
		closed: make(chan struct{}),
		events: make(chan tun.Event, 1),
	}
	t.events <- tun.EventUp
	return t
}

func (t *bridgeTUN) File() *os.File { return nil }

func (t *bridgeTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(bufs) == 0 {
		return 0, io.ErrShortBuffer
	}
	select {
	case <-t.closed:
		return 0, os.ErrClosed
	case p := <-t.fromOS:
		pkt := *p
		if offset+len(pkt) > len(bufs[0]) {
			putBuf(p)
			return 0, io.ErrShortBuffer
		}
		copy(bufs[0][offset:], pkt)
		sizes[0] = len(pkt)
		putBuf(p)
		return 1, nil
	}
}

func (t *bridgeTUN) Write(bufs [][]byte, offset int) (int, error) {
	n := 0
	for _, buf := range bufs {
		if offset > len(buf) {
			continue
		}
		p := getBuf(buf[offset:])
		select {
		case <-t.closed:
			putBuf(p)
			return n, os.ErrClosed
		case t.toOS <- p:
			n++
		default:
			// Drop when the Swift reader is behind; never block the pump.
			putBuf(p)
			n++
		}
	}
	return n, nil
}

func (t *bridgeTUN) MTU() (int, error) { return t.mtu, nil }

func (t *bridgeTUN) Name() (string, error) { return "masque", nil }

func (t *bridgeTUN) Events() <-chan tun.Event { return t.events }

func (t *bridgeTUN) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
	})
	return nil
}

func (t *bridgeTUN) BatchSize() int { return 1 }

// StartPacketBridge attaches a userspace packet queue after Dial, then
// starts forwarding. Call ReadPacket/WritePacket from the iOS extension.
func (t *Tunnel) StartPacketBridge() error {
	t.mu.Lock()
	mtu := 1400
	if t.prof != nil && t.prof.MTU > 0 {
		mtu = t.prof.MTU
	}
	if t.bridge != nil {
		t.mu.Unlock()
		return fmt.Errorf("packet bridge already attached")
	}
	br := newBridgeTUN(mtu)
	t.bridge = br
	t.mu.Unlock()
	return t.startWithDevice(br, "packet bridge ready")
}

// WritePacket injects an IP packet from the OS (NEPacketTunnelFlow).
func (t *Tunnel) WritePacket(pkt []byte) error {
	if len(pkt) == 0 {
		return nil
	}
	t.mu.Lock()
	br := t.bridge
	t.mu.Unlock()
	if br == nil {
		return fmt.Errorf("packet bridge not started")
	}
	cp := getBuf(pkt)
	select {
	case <-br.closed:
		putBuf(cp)
		return fmt.Errorf("tunnel stopped")
	case br.fromOS <- cp:
		return nil
	default:
		// Drop rather than block the Network Extension read callback.
		putBuf(cp)
		return nil
	}
}

// ReadPacket blocks until a packet should be written to the OS, or the
// tunnel is stopped (returns nil).
func (t *Tunnel) ReadPacket() []byte {
	t.mu.Lock()
	br := t.bridge
	t.mu.Unlock()
	if br == nil {
		return nil
	}
	select {
	case <-br.closed:
		return nil
	case p := <-br.toOS:
		out := append([]byte(nil), (*p)...)
		putBuf(p)
		return out
	}
}
