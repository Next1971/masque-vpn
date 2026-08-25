// Package mobile is a gomobile bridge between the shared clientcore and Android.
package mobile

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/Next1971/masque-vpn/internal/clientcore"
	"golang.zx2c4.com/wireguard/tun"
)

// Config holds connection parameters passed from Java.
type Config struct {
	Server     string
	ServerName string
	CAPath     string
	CertPath   string
	KeyPath    string
	MTU        int
}

// Callback provides status/error notifications to Java.
type Callback interface {
	OnStatus(msg string)
	OnError(msg string)
}

// Tunnel represents an active session plus a long-lived TUN.
// QUIC/CONNECT-IP may be replaced on failure; the TUN fd is not.
type Tunnel struct {
	mu       sync.Mutex
	sess     *clientcore.Session
	prof     *clientcore.Profile
	ctx      context.Context
	cancel   context.CancelFunc
	cb       Callback
	lastAddr string
	lastBits int
	started  bool
	stopped  bool
}

// AssignedAddr returns the server-assigned IPv4/IPv6 address (without prefix
// length), e.g. "10.8.0.253". Empty if no address was assigned. Used by the
// Android wrapper to configure the VpnService TUN with the correct address
// BEFORE establishing the interface (two-phase flow).
func (t *Tunnel) AssignedAddr() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess != nil && len(t.sess.AssignedPrefixes) > 0 {
		return t.sess.AssignedPrefixes[0].Addr().String()
	}
	return t.lastAddr
}

// AssignedPrefixLen returns the prefix length of the first assigned prefix
// (e.g. 32 for a /32 host route). Returns 0 if none.
func (t *Tunnel) AssignedPrefixLen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess != nil && len(t.sess.AssignedPrefixes) > 0 {
		return t.sess.AssignedPrefixes[0].Bits()
	}
	return t.lastBits
}

// UDPFd is the QUIC UDP socket. Android must protect() it and bind it to the
// current underlying network so Wi-Fi→LTE does not leave the socket on a dead path.
func (t *Tunnel) UDPFd() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess == nil {
		return -1
	}
	return t.sess.UDPFd()
}

// RTTMillis is the smoothed QUIC RTT to the VPN server in milliseconds.
// Zero if the session is down or no sample exists yet.
func (t *Tunnel) RTTMillis() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess == nil {
		return 0
	}
	d := t.sess.RTT()
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

func profileFromConfig(cfg *Config) *clientcore.Profile {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1400
	}
	return &clientcore.Profile{
		Server:     cfg.Server,
		ServerName: cfg.ServerName,
		CA:         cfg.CAPath,
		Cert:       cfg.CertPath,
		Key:        cfg.KeyPath,
		TUNName:    "",
		MTU:        mtu,
	}
}

func rememberAssigned(t *Tunnel, sess *clientcore.Session) {
	if sess == nil || len(sess.AssignedPrefixes) == 0 {
		return
	}
	t.lastAddr = sess.AssignedPrefixes[0].Addr().String()
	t.lastBits = sess.AssignedPrefixes[0].Bits()
}

// Dial establishes the CONNECT-IP session WITHOUT a TUN device. After it
// returns, read AssignedAddr()/AssignedPrefixLen(), build the platform TUN
// with that address, then call StartWithFD(fd) to attach the interface and
// begin forwarding. This is the correct flow on Android, where the TUN
// address must be known before VpnService.Builder.establish().
func Dial(cfg *Config, cb Callback) (*Tunnel, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	prof := profileFromConfig(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	sess, err := clientcore.Connect(ctx, prof, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("dial: %w", err)
	}
	if cb != nil {
		cb.OnStatus("CONNECT-IP session established")
		if len(sess.AssignedPrefixes) > 0 {
			cb.OnStatus("assigned " + sess.AssignedPrefixes[0].String())
		}
	}
	t := &Tunnel{sess: sess, prof: prof, ctx: ctx, cancel: cancel, cb: cb}
	rememberAssigned(t, sess)
	return t, nil
}

// StartWithFD attaches a TUN interface (from VpnService fd) to a tunnel
// previously created by Dial, then starts forwarding in the background.
// The fd is consumed once. If the QUIC session dies, the same TUN is kept
// and a new session is dialed; OnError is only used if the TUN itself fails.
func (t *Tunnel) StartWithFD(fd int) error {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return fmt.Errorf("tunnel already stopped")
	}
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("tunnel already started")
	}
	sess := t.sess
	cb := t.cb
	prof := t.prof
	ctx := t.ctx
	t.mu.Unlock()

	dev, name, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return fmt.Errorf("create TUN from fd %d: %w", fd, err)
	}
	if cb != nil {
		cb.OnStatus("TUN from fd ready: " + name)
	}
	sess.AttachTUN(dev)

	t.mu.Lock()
	t.started = true
	t.mu.Unlock()

	if ctx == nil || sess == nil || prof == nil {
		return fmt.Errorf("tunnel not dialed")
	}
	pump := clientcore.NewPump(sess, dev)
	go t.runPump(ctx, pump, prof, cb)
	if cb != nil {
		cb.OnStatus("forwarding started")
	}
	return nil
}

func (t *Tunnel) runPump(ctx context.Context, pump *clientcore.Pump, prof *clientcore.Profile, cb Callback) {
	err := pump.Run(ctx, func(ctx context.Context) (*clientcore.Session, error) {
		if cb != nil {
			cb.OnStatus("reconnecting")
		}
		s, err := clientcore.Connect(ctx, prof, nil)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		if t.stopped {
			t.mu.Unlock()
			s.Close()
			return nil, context.Canceled
		}
		prev := t.lastAddr
		t.sess = s
		rememberAssigned(t, s)
		newAddr := t.lastAddr
		t.mu.Unlock()
		if prev != "" && newAddr != "" && prev != newAddr {
			log.Printf("reconnect assigned %s (TUN still has %s); return path may fail until the pool gives the same /32", newAddr, prev)
		}
		if cb != nil {
			if newAddr != "" {
				cb.OnStatus("reconnected, assigned " + newAddr)
			} else {
				cb.OnStatus("reconnected")
			}
		}
		return s, nil
	})

	t.mu.Lock()
	stopped := t.stopped
	t.mu.Unlock()
	if stopped {
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) && cb != nil {
		cb.OnError(err.Error())
	}
}

// FirstAddress returns the first server-assigned address as a string.
func (t *Tunnel) FirstAddress() string {
	return t.AssignedAddr()
}

// Stop gracefully closes the tunnel. Idempotent. Does not close the TUN fd;
// the Java wrapper owns that ParcelFileDescriptor.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	if t.cancel != nil {
		t.cancel()
	}
	if t.sess != nil {
		t.sess.Close()
	}
}

// SetVerbose enables verbose diagnostic logging from the core.
func SetVerbose(on bool) { clientcore.Verbose = on }

// Connect establishes a connection using the fd from VpnService and returns a Tunnel.
func Connect(cfg *Config, fd int, cb Callback) (*Tunnel, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	prof := profileFromConfig(cfg)

	dev, name, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return nil, fmt.Errorf("create TUN from fd %d: %w", fd, err)
	}
	if cb != nil {
		cb.OnStatus("TUN from fd ready: " + name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess, err := clientcore.Connect(ctx, prof, dev)
	if err != nil {
		cancel()
		dev.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	if cb != nil {
		cb.OnStatus("CONNECT-IP session established")
		if len(sess.AssignedPrefixes) > 0 {
			cb.OnStatus("assigned " + sess.AssignedPrefixes[0].String())
		}
	}

	t := &Tunnel{sess: sess, prof: prof, ctx: ctx, cancel: cancel, cb: cb, started: true}
	rememberAssigned(t, sess)
	pump := clientcore.NewPump(sess, dev)
	go t.runPump(ctx, pump, prof, cb)
	return t, nil
}
