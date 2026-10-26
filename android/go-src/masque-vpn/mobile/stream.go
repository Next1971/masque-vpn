package mobile

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

const streamQueue = 32

// StreamWriter sends bytes on the physical TCP path. Implemented in Swift
// (NWTCPConnection). gomobile invokes it on the Go TLS/HTTP/2 thread; the
// Swift side waits for the write to finish so TLS records are not dropped.
type StreamWriter interface {
	WriteStream(p []byte) error
}

// StreamPipe is a net.Conn that Swift feeds and drains. Only Deliver and Close
// are exported to gomobile; Read/Write stay on the unexported conn.
type StreamPipe struct {
	conn *bridgeStreamConn
}

// NewStreamPipe wraps a Swift TCP writer. host/port are the MASQUE server
// (numeric IP, no DNS). The TCP session must already be connected.
func NewStreamPipe(w StreamWriter, host string, port int) (*StreamPipe, error) {
	if w == nil {
		return nil, fmt.Errorf("nil stream writer")
	}
	if host == "" || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid remote %s:%d", host, port)
	}
	remote, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("remote addr: %w", err)
	}
	localIP := net.IPv4zero
	if remote.IP.To4() == nil {
		localIP = net.IPv6unspecified
	}
	c := &bridgeStreamConn{
		writer:   w,
		remote:   remote,
		local:    &net.TCPAddr{IP: localIP, Port: 0},
		incoming: make(chan *[]byte, streamQueue),
		closed:   make(chan struct{}),
		poke:     make(chan struct{}, 1),
	}
	return &StreamPipe{conn: c}, nil
}

// Deliver is incoming TCP bytes from the Network Extension connection.
func (p *StreamPipe) Deliver(b []byte) {
	if p == nil || p.conn == nil || len(b) == 0 {
		return
	}
	p.conn.deliver(b)
}

// Close unblocks TLS reads. Idempotent.
func (p *StreamPipe) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

type bridgeStreamConn struct {
	writer    StreamWriter
	remote    net.Addr
	local     net.Addr
	incoming  chan *[]byte
	closed    chan struct{}
	closeOnce sync.Once
	poke      chan struct{}

	mu       sync.Mutex
	leftover []byte
	readDead time.Time
}

func (c *bridgeStreamConn) deliver(b []byte) {
	cp := getBuf(b)
	select {
	case <-c.closed:
		putBuf(cp)
	case c.incoming <- cp:
	}
}

func (c *bridgeStreamConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.mu.Lock()
	if n := copy(p, c.leftover); n > 0 {
		c.leftover = c.leftover[n:]
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()

	for {
		if err := c.closedErr(); err != nil {
			return 0, err
		}
		if err := c.readDeadlineErr(); err != nil {
			return 0, err
		}
		timer, timeout := c.readTimer()
		select {
		case <-c.closed:
			if timer != nil {
				timer.Stop()
			}
			return 0, net.ErrClosed
		case ptr := <-c.incoming:
			if timer != nil {
				timer.Stop()
			}
			n := copy(p, *ptr)
			if n < len(*ptr) {
				extra := append([]byte(nil), (*ptr)[n:]...)
				c.mu.Lock()
				c.leftover = extra
				c.mu.Unlock()
			}
			putBuf(ptr)
			return n, nil
		case <-timeout:
			return 0, os.ErrDeadlineExceeded
		case <-c.poke:
			if timer != nil {
				timer.Stop()
			}
		}
	}
}

func (c *bridgeStreamConn) Write(p []byte) (int, error) {
	if err := c.closedErr(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := c.writer.WriteStream(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *bridgeStreamConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *bridgeStreamConn) LocalAddr() net.Addr  { return c.local }
func (c *bridgeStreamConn) RemoteAddr() net.Addr { return c.remote }

func (c *bridgeStreamConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *bridgeStreamConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDead = t
	c.mu.Unlock()
	select {
	case c.poke <- struct{}{}:
	default:
	}
	return nil
}

func (c *bridgeStreamConn) SetWriteDeadline(time.Time) error { return nil }

func (c *bridgeStreamConn) closedErr() error {
	select {
	case <-c.closed:
		return net.ErrClosed
	default:
		return nil
	}
}

func (c *bridgeStreamConn) readDeadlineErr() error {
	c.mu.Lock()
	dl := c.readDead
	c.mu.Unlock()
	if !dl.IsZero() && !time.Now().Before(dl) {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (c *bridgeStreamConn) readTimer() (*time.Timer, <-chan time.Time) {
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
