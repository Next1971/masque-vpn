package clientcore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go"
	"github.com/yosida95/uritemplate/v3"
	"golang.zx2c4.com/wireguard/tun"
)

func dualDialAddrs(primary *net.UDPAddr, altPort int) []*net.UDPAddr {
	if primary == nil {
		return nil
	}
	out := []*net.UDPAddr{primary}
	if altPort < 1 || altPort > 65535 || altPort == primary.Port {
		return out
	}
	cp := *primary
	cp.Port = altPort
	return append(out, &cp)
}

type quicLeg struct {
	udpConn *net.UDPConn
	qconn   *quic.Conn
	addr    string
}

func (l *quicLeg) close() {
	if l == nil {
		return
	}
	if l.qconn != nil {
		_ = l.qconn.CloseWithError(0, "")
	}
	if l.udpConn != nil {
		_ = l.udpConn.Close()
	}
}

func listenAndDialQUIC(ctx context.Context, udpAddr *net.UDPAddr, tlsConf *tls.Config) (*quicLeg, error) {
	udpConn, err := listenUDPFor(udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}
	qconn, err := quic.Dial(ctx, udpConn, udpAddr, tlsConf, newQUICConfig())
	if err != nil {
		_ = udpConn.Close()
		return nil, fmt.Errorf("QUIC dial: %w", err)
	}
	log.Printf("QUIC connection established to %s", udpAddr.String())
	return &quicLeg{udpConn: udpConn, qconn: qconn, addr: udpAddr.String()}, nil
}

func raceQUIC(ctx context.Context, addrs []*net.UDPAddr, tlsConf *tls.Config) (*quicLeg, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		leg  *quicLeg
		err  error
		addr string
	}
	ch := make(chan outcome, len(addrs))
	for _, addr := range addrs {
		addr := addr
		go func() {
			leg, err := listenAndDialQUIC(ctx, addr, tlsConf)
			ch <- outcome{leg: leg, err: err, addr: addr.String()}
		}()
	}

	var errs []error
	var winner *quicLeg
	for remaining := len(addrs); remaining > 0; remaining-- {
		o := <-ch
		if o.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", o.addr, o.err))
			continue
		}
		if winner == nil {
			winner = o.leg
			cancel()
			log.Printf("dual-port: using %s", o.addr)
			continue
		}
		o.leg.close()
	}
	if winner != nil {
		return winner, nil
	}
	return nil, fmt.Errorf("QUIC dial on all ports failed: %w", errors.Join(errs...))
}

func finishCONNECTIP(ctx context.Context, p *Profile, leg *quicLeg, dev tun.Device) (*Session, error) {
	fail := func(e error) (*Session, error) {
		leg.close()
		return nil, e
	}

	template := uritemplate.MustNew(fmt.Sprintf("https://%s/vpn", p.ServerName))
	req, err := connectip.NewRequest(ctx, template)
	if err != nil {
		return fail(fmt.Errorf("connect-ip request: %w", err))
	}
	cc, err := (&connectip.Transport{}).NewClientConn(leg.qconn)
	if err != nil {
		return fail(fmt.Errorf("connect-ip client: %w", err))
	}
	ipconn, rsp, err := cc.Dial(req)
	if err != nil {
		return fail(fmt.Errorf("connect-ip dial: %w", err))
	}
	if rsp.StatusCode != http.StatusOK {
		ipconn.Close()
		return fail(fmt.Errorf("unexpected CONNECT-IP status: %d", rsp.StatusCode))
	}
	log.Printf("CONNECT-IP session established (HTTP %d) via %s", rsp.StatusCode, leg.addr)

	prefixes, err := ipconn.LocalPrefixes(ctx)
	if err != nil {
		ipconn.Close()
		return fail(fmt.Errorf("get local prefixes: %w", err))
	}
	if len(prefixes) == 0 {
		ipconn.Close()
		return fail(fmt.Errorf("server assigned no prefixes"))
	}
	log.Printf("server assigned prefixes: %v", prefixes)

	routes, err := ipconn.Routes(ctx)
	if err != nil {
		ipconn.Close()
		return fail(fmt.Errorf("get routes: %w", err))
	}
	for _, r := range routes {
		log.Printf("server advertised route: %s - %s (proto %d)", r.StartIP, r.EndIP, r.IPProtocol)
	}

	return &Session{
		udpConn:          leg.udpConn,
		qconn:            leg.qconn,
		ipconn:           ipconn,
		dev:              dev,
		AssignedPrefixes: prefixes,
		Routes:           routes,
		DialAddr:         leg.addr,
		done:             make(chan struct{}),
	}, nil
}
