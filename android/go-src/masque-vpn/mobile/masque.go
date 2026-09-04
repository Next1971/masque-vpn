// Package mobile is a gomobile bridge between the shared clientcore and
// Android / iOS wrappers.
package mobile

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"

	"github.com/Next1971/masque-vpn/internal/clientcore"
	"golang.zx2c4.com/wireguard/tun"
)

// Config holds connection parameters passed from Java/Swift.
type Config struct {
	Server     string
	ServerName string
	CAPath     string
	CertPath   string
	KeyPath    string
	MTU        int
	// BindInterface is the OS interface name for the QUIC UDP socket (iOS).
	BindInterface string
}

// Callback provides status/error notifications to Java.
type Callback interface {
	OnStatus(msg string)
	OnError(msg string)
}

// Tunnel represents an active session plus a long-lived TUN.
// QUIC/CONNECT-IP may be replaced on failure; the TUN fd is not.
type Tunnel struct {
	mu         sync.Mutex
	sess       *clientcore.Session
	prof       *clientcore.Profile
	ctx        context.Context
	cancel     context.CancelFunc
	cb         Callback
	lastAddr   string
	lastBits   int
	lastAddrV6 string
	started    bool
	stopped    bool
	bridge     *bridgeTUN
	pipe       *DatagramPipe
}

// AssignedAddr returns the server-assigned IPv4/IPv6 address (without prefix
// length), e.g. "10.8.0.253". Empty if no address was assigned. Used by the
// Android wrapper to configure the VpnService TUN with the correct address
// BEFORE establishing the interface (two-phase flow).
func (t *Tunnel) AssignedAddr() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess != nil {
		if p, ok := firstPrefix(t.sess.AssignedPrefixes, false); ok {
			return p.Addr().String()
		}
	}
	return t.lastAddr
}

// AssignedPrefixLen returns the prefix length of the first assigned IPv4 prefix
// (e.g. 32 for a /32 host route). Returns 0 if none.
func (t *Tunnel) AssignedPrefixLen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess != nil {
		if p, ok := firstPrefix(t.sess.AssignedPrefixes, false); ok {
			return p.Bits()
		}
	}
	return t.lastBits
}

// AssignedAddrV6 is the server-assigned IPv6 address without prefix length.
func (t *Tunnel) AssignedAddrV6() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sess != nil {
		if p, ok := firstPrefix(t.sess.AssignedPrefixes, true); ok {
			return p.Addr().String()
		}
	}
	return t.lastAddrV6
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

// ServerIPv4 is the VPN server's IPv4, used on iOS to exclude the QUIC
// path from the tunnel (there is no VpnService.protect).
func (t *Tunnel) ServerIPv4() string {
	t.mu.Lock()
	prof := t.prof
	t.mu.Unlock()
	if prof == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(prof.Server)
	if err != nil {
		host = prof.Server
	}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ""
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return ""
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

func profileFromConfig(cfg *Config) *clientcore.Profile {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1400
	}
	return &clientcore.Profile{
		Server:        cfg.Server,
		ServerName:    cfg.ServerName,
		CA:            cfg.CAPath,
		Cert:          cfg.CertPath,
		Key:           cfg.KeyPath,
		TUNName:       "",
		MTU:           mtu,
		BindInterface: cfg.BindInterface,
	}
}

func rememberAssigned(t *Tunnel, sess *clientcore.Session) {
	if sess == nil {
		return
	}
	if p, ok := firstPrefix(sess.AssignedPrefixes, false); ok {
		t.lastAddr = p.Addr().String()
		t.lastBits = p.Bits()
	}
	if p, ok := firstPrefix(sess.AssignedPrefixes, true); ok {
		t.lastAddrV6 = p.Addr().String()
	}
}

func firstPrefix(prefixes []netip.Prefix, want6 bool) (netip.Prefix, bool) {
	for _, p := range prefixes {
		is6 := p.Addr().Is6() && !p.Addr().Is4In6()
		if is6 == want6 {
			return p, true
		}
	}
	return netip.Prefix{}, false
}

// Dial establishes the CONNECT-IP session WITHOUT a TUN device. After it
// returns, read AssignedAddr()/AssignedPrefixLen(), configure the platform
// interface, then attach it with StartWithFD (Android) or StartPacketBridge
// (iOS) and begin forwarding.
func Dial(cfg *Config, cb Callback) (t *Tunnel, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dial panic: %v", r)
			t = nil
		}
	}()
	return dial(cfg, cb, nil)
}

// DialWithPipe is Dial using a Swift UDP session as the QUIC PacketConn.
// iOS must open NEPacketTunnelProvider.createUDPSession (or NWConnection)
// before calling this, and must not complete startTunnel until it returns.
func DialWithPipe(cfg *Config, cb Callback, pipe *DatagramPipe) (t *Tunnel, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dial panic: %v", r)
			t = nil
		}
	}()
	if pipe == nil || pipe.conn == nil {
		return nil, fmt.Errorf("nil datagram pipe")
	}
	return dial(cfg, cb, pipe)
}

func dial(cfg *Config, cb Callback, pipe *DatagramPipe) (*Tunnel, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	prof := profileFromConfig(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	sess, err := connectSession(ctx, prof, pipe)
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
	t := &Tunnel{sess: sess, prof: prof, ctx: ctx, cancel: cancel, cb: cb, pipe: pipe}
	rememberAssigned(t, sess)
	return t, nil
}

func connectSession(ctx context.Context, prof *clientcore.Profile, pipe *DatagramPipe) (*clientcore.Session, error) {
	if pipe != nil && pipe.conn != nil {
		return clientcore.ConnectWithPacketConn(ctx, prof, nil, pipe.conn)
	}
	return clientcore.Connect(ctx, prof, nil)
}

func (t *Tunnel) startWithDevice(dev tun.Device, readyMsg string) error {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		_ = dev.Close()
		return fmt.Errorf("tunnel already stopped")
	}
	if t.started {
		t.mu.Unlock()
		_ = dev.Close()
		return fmt.Errorf("tunnel already started")
	}
	sess := t.sess
	cb := t.cb
	prof := t.prof
	ctx := t.ctx
	t.mu.Unlock()

	if ctx == nil || sess == nil || prof == nil {
		_ = dev.Close()
		return fmt.Errorf("tunnel not dialed")
	}
	if cb != nil && readyMsg != "" {
		cb.OnStatus(readyMsg)
	}
	sess.AttachTUN(dev)

	t.mu.Lock()
	t.started = true
	t.mu.Unlock()

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
		s, err := connectSession(ctx, prof, t.pipe)
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
		newAddr := ""
		if len(s.AssignedPrefixes) > 0 {
			newAddr = s.AssignedPrefixes[0].Addr().String()
		}
		cancel := t.cancel
		if prev != "" && newAddr != "" && prev != newAddr {
			t.mu.Unlock()
			log.Printf("assigned address changed %s -> %s; requesting TUN rebuild", prev, newAddr)
			s.Close()
			if cb != nil {
				cb.OnStatus("assigned-ip-changed")
			}
			if cancel != nil {
				cancel()
			}
			return nil, context.Canceled
		}
		t.sess = s
		rememberAssigned(t, s)
		t.mu.Unlock()
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

// Stop gracefully closes the tunnel. Idempotent. Does not close an Android
// TUN fd; the Java wrapper owns that ParcelFileDescriptor. On iOS it closes
// the packet bridge so ReadPacket unblocks.
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
	if t.pipe != nil {
		_ = t.pipe.Close()
	}
	if t.bridge != nil {
		_ = t.bridge.Close()
	}
}

// SetVerbose enables verbose diagnostic logging from the core.
func SetVerbose(on bool) { clientcore.Verbose = on }
