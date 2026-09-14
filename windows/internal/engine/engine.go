//go:build windows

package engine

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/tun"
	"masque-client/internal/clientcore"
	"masque-client/internal/ipc"
	"masque-client/internal/store"
	"masque-client/internal/winnet"
)

var (
	ksPlaceholderV4 = netip.MustParsePrefix("10.8.0.254/32")
	ksPlaceholderV6 = netip.MustParsePrefix("fd00:8::fe/128")
)

type Status struct {
	State       string
	Detail      string
	Configured  bool
	Autoconnect bool
	KillSwitch  bool
	AssignedIP  string
	RTTMs       int64
}

type Engine struct {
	mu       sync.Mutex
	status   Status
	cancel   context.CancelFunc
	hub      *ipc.Hub
	onChange func(Status)
	pump     *clientcore.Pump
}

func New(hub *ipc.Hub) *Engine {
	s := store.LoadSettings()
	return &Engine{
		hub: hub,
		status: Status{
			State:       ipc.StateDisconnected,
			Configured:  store.Configured(),
			Autoconnect: s.Autoconnect,
			KillSwitch:  s.KillSwitch,
		},
	}
}

func (e *Engine) Snapshot() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	st := store.LoadSettings()
	s.Configured = store.Configured()
	s.Autoconnect = st.Autoconnect
	s.KillSwitch = st.KillSwitch
	s.RTTMs = 0
	if e.pump != nil && (s.State == ipc.StateConnected || s.State == ipc.StateReconnecting) {
		if d := e.pump.RTT(); d > 0 {
			s.RTTMs = d.Milliseconds()
		}
	}
	return s
}

func (e *Engine) set(state, detail, ip string) {
	st := store.LoadSettings()
	e.mu.Lock()
	e.status.State = state
	e.status.Detail = detail
	if ip != "" {
		e.status.AssignedIP = ip
	}
	if state == ipc.StateDisconnected {
		e.status.AssignedIP = ""
	}
	e.status.Configured = store.Configured()
	e.status.Autoconnect = st.Autoconnect
	e.status.KillSwitch = st.KillSwitch
	snap := e.status
	cb := e.onChange
	hub := e.hub
	e.mu.Unlock()
	if cb != nil {
		cb(snap)
	}
	if hub != nil {
		hub.Broadcast(ipc.Response{
			OK: true, State: snap.State, Detail: snap.Detail,
			Configured: snap.Configured, Autoconnect: snap.Autoconnect,
			KillSwitch: snap.KillSwitch, AssignedIP: snap.AssignedIP,
			RTTMs: snap.RTTMs,
		})
	}
}

func (e *Engine) Import(text, filename, ca, cert, key string) error {
	e.mu.Lock()
	busy := e.cancel != nil
	e.mu.Unlock()
	if busy {
		return fmt.Errorf("disconnect before importing a new profile")
	}
	if err := store.Import(text, filename, ca, cert, key); err != nil {
		return err
	}
	e.set(ipc.StateDisconnected, "profile imported", "")
	return nil
}

func (e *Engine) SetAutoconnect(v bool) error {
	if err := store.SetAutoconnect(v); err != nil {
		return err
	}
	s := e.Snapshot()
	e.set(s.State, s.Detail, s.AssignedIP)
	return nil
}

func (e *Engine) SetKillSwitch(v bool) error {
	if err := store.SetKillSwitch(v); err != nil {
		return err
	}
	s := e.Snapshot()
	e.set(s.State, s.Detail, s.AssignedIP)
	return nil
}

func (e *Engine) MaybeAutoconnect() {
	if !store.LoadSettings().Autoconnect || !store.Configured() {
		return
	}
	if err := e.Connect(); err != nil {
		log.Printf("autoconnect failed: %v", err)
	}
}

func (e *Engine) Connect() error {
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	if !store.Configured() {
		e.mu.Unlock()
		return fmt.Errorf("no profile imported")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.mu.Unlock()
	e.set(ipc.StateConnecting, "dialing server", "")
	go e.loop(ctx)
	return nil
}

func (e *Engine) Disconnect() {
	e.mu.Lock()
	cancel := e.cancel
	pump := e.pump
	e.mu.Unlock()
	// Cancel first so Pump.Run does not reconnect when the session closes.
	if cancel != nil {
		cancel()
	}
	pump.Close()
}

func (e *Engine) loop(ctx context.Context) {
	defer func() {
		e.mu.Lock()
		e.cancel = nil
		e.mu.Unlock()
		if ctx.Err() != nil {
			e.set(ipc.StateDisconnected, "disconnected", "")
		}
	}()

	if store.LoadSettings().KillSwitch {
		if err := e.loopKillSwitch(ctx); err != nil && ctx.Err() == nil {
			e.set(ipc.StateError, err.Error(), "")
		}
		return
	}
	if err := e.runOnce(ctx); err != nil && ctx.Err() == nil {
		e.set(ipc.StateError, err.Error(), "")
	}
}

// loopKillSwitch brings up TUN routes before the first successful dial and
// leaves them in place until the user disconnects, so traffic cannot fall
// back to the physical default gateway.
func (e *Engine) loopKillSwitch(ctx context.Context) error {
	prof, err := store.Load()
	if err != nil {
		return err
	}
	dev, err := tun.CreateTUN(prof.TUNName, prof.MTU)
	if err != nil {
		return fmt.Errorf("create TUN: %w", err)
	}
	name, _ := dev.Name()
	defer dev.Close()

	if err := winnet.IfUp(name, ksPlaceholderV4); err != nil {
		return fmt.Errorf("interface up: %w", err)
	}
	cleanup, err := winnet.SetupFullRoute(name, prof.Server, ksPlaceholderV4.Addr(), prof.DNS)
	if err != nil {
		return fmt.Errorf("routes: %w", err)
	}
	defer cleanup()
	if err := winnet.IfUpIPv6(name, ksPlaceholderV6); err != nil {
		log.Printf("warn: IPv6 placeholder on TUN: %v", err)
	} else if c6, err := winnet.SetupIPv6Default(name, ksPlaceholderV6.Addr()); err != nil {
		log.Printf("warn: IPv6 default via TUN: %v", err)
	} else {
		defer c6()
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.set(ipc.StateConnecting, "kill switch on; dialing server", "")
		err := e.pumpOn(ctx, prof, dev, name, true)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := "kill switch: internet blocked until the tunnel returns"
		if err != nil {
			msg = fmt.Sprintf("%s (%v)", msg, err)
		}
		e.set(ipc.StateReconnecting, msg, "")
		if !sleepCtx(ctx, backoff) {
			return ctx.Err()
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (e *Engine) runOnce(ctx context.Context) error {
	prof, err := store.Load()
	if err != nil {
		return err
	}
	dev, err := tun.CreateTUN(prof.TUNName, prof.MTU)
	if err != nil {
		return fmt.Errorf("create TUN: %w", err)
	}
	name, _ := dev.Name()
	defer dev.Close()
	return e.pumpOn(ctx, prof, dev, name, false)
}

func (e *Engine) pumpOn(ctx context.Context, prof *clientcore.Profile, dev tun.Device, name string, routesAlreadyOn bool) error {
	sess, err := clientcore.Connect(ctx, prof, nil)
	if err != nil {
		return err
	}
	if len(sess.AssignedPrefixes) == 0 {
		sess.Close()
		return fmt.Errorf("server assigned no address")
	}
	v4, v6 := clientcore.SplitAssigned(sess.AssignedPrefixes)
	if !v4.IsValid() {
		sess.Close()
		return fmt.Errorf("server assigned no IPv4 address")
	}
	clientAddr := v4
	if err := winnet.IfUp(name, clientAddr); err != nil {
		sess.Close()
		return fmt.Errorf("interface up: %w", err)
	}
	var cleanup, cleanup6 func()
	if !routesAlreadyOn {
		c, err := winnet.SetupFullRoute(name, prof.Server, clientAddr.Addr(), prof.DNS)
		if err != nil {
			sess.Close()
			return fmt.Errorf("routes: %w", err)
		}
		cleanup = c
	}
	if v6.IsValid() {
		if err := winnet.IfUpIPv6(name, v6); err != nil {
			sess.Close()
			if cleanup != nil {
				cleanup()
			}
			return fmt.Errorf("IPv6 interface up: %w", err)
		}
		if !routesAlreadyOn {
			c6, err := winnet.SetupIPv6Default(name, v6.Addr())
			if err != nil {
				sess.Close()
				if cleanup != nil {
					cleanup()
				}
				return fmt.Errorf("IPv6 routes: %w", err)
			}
			cleanup6 = c6
		}
	}
	defer func() {
		if cleanup6 != nil {
			cleanup6()
		}
		if cleanup != nil {
			cleanup()
		}
	}()

	e.set(ipc.StateConnected, "connected", clientAddr.Addr().String())

	pump := clientcore.NewPump(sess, dev)
	e.mu.Lock()
	e.pump = pump
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.pump = nil
		e.mu.Unlock()
	}()
	pump.OnRedial = func() {
		e.set(ipc.StateReconnecting, "connection dropped by the network; reconnecting", clientAddr.Addr().String())
	}
	err = pump.Run(ctx, func(ctx context.Context) (*clientcore.Session, error) {
		s, err := clientcore.Connect(ctx, prof, nil)
		if err != nil {
			return nil, err
		}
		e.set(ipc.StateConnected, "reconnected", clientAddr.Addr().String())
		return s, nil
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("session ended")
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
