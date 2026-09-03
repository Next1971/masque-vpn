//go:build android || linux

package mobile

import (
	"context"
	"fmt"

	"github.com/Next1971/masque-vpn/internal/clientcore"
	"golang.zx2c4.com/wireguard/tun"
)

// StartWithFD attaches a TUN interface (from VpnService fd) to a tunnel
// previously created by Dial, then starts forwarding in the background.
// The fd is consumed once. If the QUIC session dies, the same TUN is kept
// and a new session is dialed; OnError is only used if the TUN itself fails.
func (t *Tunnel) StartWithFD(fd int) error {
	dev, name, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return fmt.Errorf("create TUN from fd %d: %w", fd, err)
	}
	return t.startWithDevice(dev, "TUN from fd ready: "+name)
}

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
