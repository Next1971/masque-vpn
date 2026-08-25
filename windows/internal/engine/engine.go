//go:build windows

package engine

import (
	"context"
	"fmt"
	"log"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
	"masque-client/internal/clientcore"
	"masque-client/internal/ipc"
	"masque-client/internal/store"
	"masque-client/internal/winnet"
)

type Status struct {
	State       string
	Detail      string
	Configured  bool
	Autoconnect bool
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
	return &Engine{
		hub: hub,
		status: Status{
			State:       ipc.StateDisconnected,
			Configured:  store.Configured(),
			Autoconnect: store.LoadSettings().Autoconnect,
		},
	}
}

func (e *Engine) Snapshot() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.Configured = store.Configured()
	s.Autoconnect = store.LoadSettings().Autoconnect
	s.RTTMs = 0
	if e.pump != nil && (s.State == ipc.StateConnected || s.State == ipc.StateReconnecting) {
		if d := e.pump.RTT(); d > 0 {
			s.RTTMs = d.Milliseconds()
		}
	}
	return s
}

func (e *Engine) set(state, detail, ip string) {
	e.mu.Lock()
	e.status.State = state
	e.status.Detail = detail
	if ip != "" {
		e.status.AssignedIP = ip
	}
	if state == ipc.StateDisconnected && state != ipc.StateError {
		e.status.AssignedIP = ""
	}
	e.status.Configured = store.Configured()
	e.status.Autoconnect = store.LoadSettings().Autoconnect
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
			Configured: snap.Configured, Autoconnect: snap.Autoconnect, AssignedIP: snap.AssignedIP,
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
	e.cancel = nil
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
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

	prof, err := store.Load()
	if err != nil {
		e.set(ipc.StateError, err.Error(), "")
		return
	}

	dev, err := tun.CreateTUN(prof.TUNName, prof.MTU)
	if err != nil {
		e.set(ipc.StateError, fmt.Sprintf("create TUN: %v", err), "")
		return
	}
	name, _ := dev.Name()
	defer dev.Close()

	sess, err := clientcore.Connect(ctx, prof, nil)
	if err != nil {
		e.set(ipc.StateError, err.Error(), "")
		return
	}
	if len(sess.AssignedPrefixes) == 0 {
		sess.Close()
		e.set(ipc.StateError, "server assigned no address", "")
		return
	}
	clientAddr := sess.AssignedPrefixes[0]
	if err := winnet.IfUp(name, clientAddr); err != nil {
		sess.Close()
		e.set(ipc.StateError, fmt.Sprintf("interface up: %v", err), "")
		return
	}
	cleanup, err := winnet.SetupFullRoute(name, prof.Server, clientAddr.Addr(), prof.DNS)
	if err != nil {
		sess.Close()
		e.set(ipc.StateError, fmt.Sprintf("routes: %v", err), "")
		return
	}
	defer cleanup()

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
	if ctx.Err() == nil && err != nil {
		e.set(ipc.StateError, err.Error(), "")
	}
}
