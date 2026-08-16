// Package mobile is a gomobile bridge between the shared clientcore and Android.
package mobile

import (
"context"
"fmt"
"sync"

"github.com/Next1971/masque-vpn-mvp/internal/clientcore"
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

// Tunnel represents an active session.
type Tunnel struct {
mu      sync.Mutex
sess    *clientcore.Session
cancel  context.CancelFunc
cb      Callback
started bool
stopped bool
}

// AssignedAddr returns the server-assigned IPv4/IPv6 address (without prefix
// length), e.g. "10.8.0.253". Empty if no address was assigned. Used by the
// Android wrapper to configure the VpnService TUN with the correct address
// BEFORE establishing the interface (two-phase flow).
func (t *Tunnel) AssignedAddr() string {
t.mu.Lock()
defer t.mu.Unlock()
if t.sess == nil || len(t.sess.AssignedPrefixes) == 0 {
return ""
}
return t.sess.AssignedPrefixes[0].Addr().String()
}

// AssignedPrefixLen returns the prefix length of the first assigned prefix
// (e.g. 32 for a /32 host route). Returns 0 if none.
func (t *Tunnel) AssignedPrefixLen() int {
t.mu.Lock()
defer t.mu.Unlock()
if t.sess == nil || len(t.sess.AssignedPrefixes) == 0 {
return 0
}
return t.sess.AssignedPrefixes[0].Bits()
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
prof := &clientcore.Profile{
Server:     cfg.Server,
ServerName: cfg.ServerName,
CA:         cfg.CAPath,
Cert:       cfg.CertPath,
Key:        cfg.KeyPath,
TUNName:    "",
MTU:        cfg.MTU,
}
if prof.MTU == 0 {
prof.MTU = 1400
}
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
return &Tunnel{sess: sess, cancel: cancel, cb: cb}, nil
}

// StartWithFD attaches a TUN interface (from VpnService fd) to a tunnel
// previously created by Dial, then starts forwarding in the background.
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

ctx := context.Background()
go func() {
err := sess.Run(ctx)
if err != nil && cb != nil {
cb.OnError(err.Error())
}
t.Stop()
}()
if cb != nil {
cb.OnStatus("forwarding started")
}
return nil
}

// FirstAddress returns the first server-assigned address as a string.
func (t *Tunnel) FirstAddress() string {
t.mu.Lock()
defer t.mu.Unlock()
if t.sess == nil || len(t.sess.AssignedPrefixes) == 0 {
return ""
}
return t.sess.AssignedPrefixes[0].Addr().String()
}

// Stop gracefully closes the tunnel. Idempotent.
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
mtu := cfg.MTU
if mtu == 0 {
mtu = 1400
}

dev, name, err := tun.CreateUnmonitoredTUNFromFD(fd)
if err != nil {
return nil, fmt.Errorf("create TUN from fd %d: %w", fd, err)
}
if cb != nil {
cb.OnStatus("TUN from fd ready: " + name)
}

prof := &clientcore.Profile{
Server:     cfg.Server,
ServerName: cfg.ServerName,
CA:         cfg.CAPath,
Cert:       cfg.CertPath,
Key:        cfg.KeyPath,
TUNName:    name,
MTU:        mtu,
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

t := &Tunnel{sess: sess, cancel: cancel}

go func() {
err := sess.Run(ctx)
if err != nil && cb != nil {
cb.OnError(err.Error())
}
t.Stop()
}()

return t, nil
}