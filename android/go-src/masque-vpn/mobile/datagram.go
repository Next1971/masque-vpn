package mobile

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

const datagramQueue = 256

// DatagramWriter sends one UDP payload on the physical path. Implemented in
// Swift (NWUDPSession / NWConnection). Must return immediately: gomobile
// invokes it on the Go QUIC thread.
type DatagramWriter interface {
	WriteDatagram(p []byte) error
}

// DatagramPipe is a net.PacketConn that Swift feeds and drains. Only Deliver
// and Close are exported to gomobile; ReadFrom/WriteTo stay on the unexported
// conn so bind does not see net.Addr.
type DatagramPipe struct {
	conn *bridgePacketConn
}

// NewDatagramPipe wraps a Swift UDP writer as the QUIC PacketConn. host/port
// are the MASQUE server (numeric IP, no DNS).
func NewDatagramPipe(w DatagramWriter, host string, port int) (*DatagramPipe, error) {
	if w == nil {
		return nil, fmt.Errorf("nil datagram writer")
	}
	if host == "" || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid remote %s:%d", host, port)
	}
	remote, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("remote addr: %w", err)
	}
	c := &bridgePacketConn{
		writer:   w,
		remote:   remote,
		local:    &net.UDPAddr{IP: net.IPv4zero, Port: 0},
		incoming: make(chan []byte, datagramQueue),
		closed:   make(chan struct{}),
		poke:     make(chan struct{}, 1),
	}
	return &DatagramPipe{conn: c}, nil
}

// Deliver is one incoming UDP datagram from the Network Extension session.
func (p *DatagramPipe) Deliver(b []byte) {
	if p == nil || p.conn == nil || len(b) == 0 {
		return
	}
	p.conn.deliver(b)
}

// Close unblocks QUIC reads. Idempotent.
func (p *DatagramPipe) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

type bridgePacketConn struct {
	writer    DatagramWriter
	remote    net.Addr
	local     net.Addr
	incoming  chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	poke      chan struct{}

	mu       sync.Mutex
	readDead time.Time
}

func (c *bridgePacketConn) deliver(b []byte) {
	cp := append([]byte(nil), b...)
	select {
	case <-c.closed:
	case c.incoming <- cp:
	default:
		// Drop only if Swift is flooding faster than quic-go can read.
	}
}

func (c *bridgePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		if err := c.closedErr(); err != nil {
			return 0, nil, err
		}
		if err := c.readDeadlineErr(); err != nil {
			return 0, nil, err
		}
		timer, timeout := c.readTimer()
		select {
		case <-c.closed:
			if timer != nil {
				timer.Stop()
			}
			return 0, nil, net.ErrClosed
		case pkt := <-c.incoming:
			if timer != nil {
				timer.Stop()
			}
			n := copy(p, pkt)
			return n, c.remote, nil
		case <-timeout:
			return 0, nil, os.ErrDeadlineExceeded
		case <-c.poke:
			if timer != nil {
				timer.Stop()
			}
		}
	}
}

func (c *bridgePacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	if err := c.closedErr(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Copy: Swift write is async and quic-go reuses this buffer.
	cp := append([]byte(nil), p...)
	if err := c.writer.WriteDatagram(cp); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *bridgePacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *bridgePacketConn) LocalAddr() net.Addr  { return c.local }
func (c *bridgePacketConn) RemoteAddr() net.Addr { return c.remote }

func (c *bridgePacketConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *bridgePacketConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDead = t
	c.mu.Unlock()
	select {
	case c.poke <- struct{}{}:
	default:
	}
	return nil
}

func (c *bridgePacketConn) SetWriteDeadline(time.Time) error { return nil }

func (c *bridgePacketConn) closedErr() error {
	select {
	case <-c.closed:
		return net.ErrClosed
	default:
		return nil
	}
}

func (c *bridgePacketConn) readDeadlineErr() error {
	c.mu.Lock()
	dl := c.readDead
	c.mu.Unlock()
	if !dl.IsZero() && !time.Now().Before(dl) {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (c *bridgePacketConn) readTimer() (*time.Timer, <-chan time.Time) {
	c.mu.Lock()
	dl := c.readDead
	c.mu.Unlock()
	if dl.IsZero() {
		return nil, nil
	}
	d := time.Until(dl)
	if d <= 0 {
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return nil, ch
	}
	t := time.NewTimer(d)
	return t, t.C
}
