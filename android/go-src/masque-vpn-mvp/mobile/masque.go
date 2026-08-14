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
stopped bool
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