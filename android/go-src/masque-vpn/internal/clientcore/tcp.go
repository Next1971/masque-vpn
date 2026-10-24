package clientcore

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	"golang.org/x/net/http2"
	"golang.zx2c4.com/wireguard/tun"
)

const tcpHandshakeTimeout = 20 * time.Second

// tcpIP is the HTTP/2 CONNECT-IP packet pipe used when the app selects TCP.
type tcpIP struct {
	body       io.ReadCloser
	pw         *io.PipeWriter
	bw         *bufio.Writer
	tcp        *net.TCPConn
	mu         sync.Mutex
	wmu        sync.Mutex
	pkts       chan []byte
	done       chan struct{}
	ready      chan struct{}
	readyOnce  sync.Once
	closeOnce  sync.Once
	err        error
	gotAssign  bool
	gotRoutes  bool
	assigned   []netip.Prefix
	routes     []connectip.IPRoute
	h2         *http2.ClientConn
	rttNS      atomic.Int64
	pingLogged atomic.Bool
}

// connectTCP opens HTTP/2 CONNECT-IP on the same host and ports as UDP
// (profile port, then alt_port). The first handshake that assigns an address wins.
func connectTCP(ctx context.Context, p *Profile, dev tun.Device) (*Session, error) {
	addrs, err := resolveDialAddrsCtx(ctx, p.Server, p.AltPort)
	if err != nil {
		return nil, err
	}
	var last error
	for _, addr := range addrs {
		sess, err := connectTCPAddr(ctx, p, dev, addr.String())
		if err == nil {
			return sess, nil
		}
		last = err
		log.Printf("tcp %s: %v", addr, err)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if last == nil {
		return nil, fmt.Errorf("tcp connect-ip: no address for %s", p.Server)
	}
	return nil, last
}

func connectTCPAddr(ctx context.Context, p *Profile, dev tun.Device, dialAddr string) (*Session, error) {
	tlsConf, err := buildTLSConfig(p)
	if err != nil {
		return nil, err
	}
	tlsConf = tlsConf.Clone()
	tlsConf.NextProtos = []string{"h2"}

	dialer := net.Dialer{Timeout: tcpHandshakeTimeout}
	raw, err := dialer.DialContext(ctx, "tcp", dialAddr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial %s: %w", dialAddr, err)
	}
	tcp, ok := raw.(*net.TCPConn)
	if !ok {
		raw.Close()
		return nil, fmt.Errorf("tcp dial %s: not a TCP socket", dialAddr)
	}
	if err := tcp.SetDeadline(time.Now().Add(tcpHandshakeTimeout)); err != nil {
		tcp.Close()
		return nil, err
	}

	tc := tls.Client(tcp, tlsConf)
	if err := tc.HandshakeContext(ctx); err != nil {
		tcp.Close()
		return nil, fmt.Errorf("tcp tls: %w", err)
	}

	h2conn, err := (&http2.Transport{}).NewClientConn(tc)
	if err != nil {
		tcp.Close()
		return nil, fmt.Errorf("http2 conn: %w", err)
	}

	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, "https://"+p.ServerName+"/vpn", pr)
	if err != nil {
		pw.Close()
		tcp.Close()
		return nil, err
	}
	req.ContentLength = -1
	req.Header[":protocol"] = []string{"connect-ip"}
	req.Header.Set("Capsule-Protocol", "?1")

	resp, err := h2conn.RoundTrip(req)
	if err != nil {
		pw.Close()
		tcp.Close()
		return nil, fmt.Errorf("tcp connect-ip: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		pw.Close()
		tcp.Close()
		return nil, fmt.Errorf("tcp connect-ip status %d", resp.StatusCode)
	}

	ip := &tcpIP{
		body:  resp.Body,
		pw:    pw,
		bw:    bufio.NewWriter(pw),
		tcp:   tcp,
		h2:    h2conn,
		pkts:  make(chan []byte, 32),
		done:  make(chan struct{}),
		ready: make(chan struct{}),
	}
	go ip.readLoop()

	select {
	case <-ip.ready:
	case <-ctx.Done():
		ip.Close()
		return nil, ctx.Err()
	case <-time.After(tcpHandshakeTimeout):
		ip.Close()
		return nil, fmt.Errorf("tcp connect-ip: no address assignment from %s", dialAddr)
	}
	_ = tcp.SetDeadline(time.Time{})

	ip.mu.Lock()
	assigned := append([]netip.Prefix(nil), ip.assigned...)
	routes := append([]connectip.IPRoute(nil), ip.routes...)
	dialErr := ip.err
	ip.mu.Unlock()
	if dialErr != nil && len(assigned) == 0 {
		ip.Close()
		return nil, fmt.Errorf("tcp connect-ip: %w", dialErr)
	}
	if len(assigned) == 0 {
		ip.Close()
		return nil, fmt.Errorf("tcp connect-ip: server assigned no prefixes")
	}
	log.Printf("CONNECT-IP session established via tcp %s, prefixes %v", dialAddr, assigned)
	go ip.loopRTT()

	return &Session{
		tcpConn:          tcp,
		ipconn:           ip,
		dev:              dev,
		AssignedPrefixes: assigned,
		Routes:           routes,
		DialAddr:         dialAddr,
		done:             make(chan struct{}),
	}, nil
}

func (c *tcpIP) signalReady() {
	c.mu.Lock()
	ready := c.gotAssign && c.gotRoutes
	c.mu.Unlock()
	if ready {
		c.readyOnce.Do(func() { close(c.ready) })
	}
}

func (c *tcpIP) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	c.readyOnce.Do(func() { close(c.ready) })
}

func (c *tcpIP) readLoop() {
	defer close(c.pkts)
	br := bufio.NewReader(c.body)
	for {
		typ, val, err := readCapsule(br)
		if err != nil {
			c.fail(err)
			return
		}
		switch typ {
		case capsuleDatagram:
			pkt, err := parseDatagram(val)
			if err != nil {
				c.fail(err)
				return
			}
			if pkt == nil {
				continue
			}
			select {
			case <-c.done:
				return
			case c.pkts <- pkt:
			}
		case capsuleAddressAssign:
			pfx, err := parseAddressAssign(val)
			if err != nil {
				c.fail(err)
				return
			}
			c.mu.Lock()
			c.assigned = pfx
			c.gotAssign = len(pfx) > 0
			c.mu.Unlock()
			c.signalReady()
		case capsuleRouteAdvertisement:
			rts, err := parseRouteAdvertisement(val)
			if err != nil {
				c.fail(err)
				return
			}
			out := make([]connectip.IPRoute, len(rts))
			for i, rt := range rts {
				out[i] = connectip.IPRoute{StartIP: rt.Start, EndIP: rt.End, IPProtocol: rt.IPProtocol}
			}
			c.mu.Lock()
			c.routes = out
			c.gotRoutes = true
			c.mu.Unlock()
			c.signalReady()
		}
	}
}

func (c *tcpIP) ReadPacket(buf []byte) (int, error) {
	select {
	case <-c.done:
		return 0, net.ErrClosed
	case pkt, ok := <-c.pkts:
		if !ok {
			return 0, io.EOF
		}
		if len(pkt) > len(buf) {
			return 0, fmt.Errorf("packet %d exceeds buffer %d", len(pkt), len(buf))
		}
		return copy(buf, pkt), nil
	}
}

func (c *tcpIP) WritePacket(pkt []byte) ([]byte, error) {
	frame := appendDatagram(pkt)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.bw.Write(frame); err != nil {
		return nil, err
	}
	if err := c.bw.Flush(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (c *tcpIP) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		c.readyOnce.Do(func() { close(c.ready) })
		if c.pw != nil {
			_ = c.pw.Close()
		}
		if c.body != nil {
			_ = c.body.Close()
		}
		if c.tcp != nil {
			_ = c.tcp.Close()
		}
	})
	return nil
}

const tcpRTTInterval = 2 * time.Second

// loopRTT samples HTTP/2 PING so the UI can show a ping while QUIC stats
// are absent. It stops when the session closes.
func (c *tcpIP) loopRTT() {
	c.sampleRTT()
	tick := time.NewTicker(tcpRTTInterval)
	defer tick.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-tick.C:
			c.sampleRTT()
		}
	}
}

func (c *tcpIP) sampleRTT() {
	if c.h2 == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	start := time.Now()
	if err := c.h2.Ping(ctx); err != nil {
		if c.pingLogged.CompareAndSwap(false, true) {
			log.Printf("tcp ping: %v", err)
		}
		return
	}
	c.noteRTT(time.Since(start))
}

func (c *tcpIP) noteRTT(sample time.Duration) {
	if sample < time.Millisecond {
		sample = time.Millisecond
	}
	for {
		prev := c.rttNS.Load()
		next := int64(sample)
		if prev > 0 {
			next = (7*prev + int64(sample)) / 8
			if next < int64(time.Millisecond) {
				next = int64(time.Millisecond)
			}
		}
		if c.rttNS.CompareAndSwap(prev, next) {
			if prev == 0 {
				log.Printf("tcp rtt %d ms", time.Duration(next).Milliseconds())
			}
			return
		}
	}
}

func (c *tcpIP) rtt() time.Duration {
	if c == nil {
		return 0
	}
	return time.Duration(c.rttNS.Load())
}
