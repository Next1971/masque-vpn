package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"

	"golang.zx2c4.com/wireguard/tun"
)

const (
	h2ConnectProtocol = "connect-ip"
	h2CapsuleHeader   = "Capsule-Protocol"
	h2CapsuleValue    = "?1"
	h2RequestPath     = "/vpn"
)

// tcpConnect carries one HTTP/2 CONNECT-IP session into the same pool and TUN
// as the QUIC listener.
type tcpConnect struct {
	blocked        map[string]struct{}
	pool, poolV6   *IPPool
	assign         netip.Prefix
	route, routeV6 netip.Prefix
	tunDev         tun.Device
	router         *Router
	mtu            int
	live           *sessionIndex
}

func (h tcpConnect) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Header.Get(":protocol") != h2ConnectProtocol || r.URL.Path != h2RequestPath {
		http.Error(w, "connect-ip required", http.StatusNotImplemented)
		return
	}
	if r.Header.Get(h2CapsuleHeader) != h2CapsuleValue {
		http.Error(w, "Capsule-Protocol: ?1 required", http.StatusBadRequest)
		return
	}

	cn := ""
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		cn = r.TLS.PeerCertificates[0].Subject.CommonName
		log.Printf("TCP client authenticated via mTLS: CN=%s", cn)
	}
	if cn != "" {
		if _, deny := h.blocked[cn]; deny {
			log.Printf("rejected blocked CN=%s", cn)
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	w.Header().Set(h2CapsuleHeader, h2CapsuleValue)
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		log.Printf("TCP flush response: %v", err)
		return
	}

	sess := newTCPSession(r.Body, w, rc.Flush, func() { _ = r.Body.Close() })
	defer sess.Close()
	h.live.take(cn, sess)
	defer h.live.drop(cn, sess)
	log.Printf("CONNECT-IP session ESTABLISHED (HTTP/2) from %s", r.RemoteAddr)

	clientAddr := h.assign
	if h.pool != nil {
		alloc, lease, err := h.pool.AllocateFor(cn)
		if err != nil {
			log.Printf("cannot allocate address: %v", err)
			return
		}
		clientAddr = alloc
		log.Printf("allocated %s for CN=%q from pool (%d left)", clientAddr, cn, h.pool.Available())
		defer func() {
			h.pool.Release(cn, lease, alloc)
			log.Printf("released %s back to pool (%d available)", alloc, h.pool.Available())
		}()
	}
	var clientAddrV6 netip.Prefix
	if h.poolV6 != nil {
		alloc6, lease6, err := h.poolV6.AllocateFor(cn)
		if err != nil {
			log.Printf("cannot allocate IPv6 address: %v", err)
			return
		}
		clientAddrV6 = alloc6
		log.Printf("allocated %s for CN=%q from IPv6 pool", clientAddrV6, cn)
		defer func() {
			h.poolV6.Release(cn, lease6, alloc6)
			log.Printf("released %s back to IPv6 pool", alloc6)
		}()
	}

	assigned := []netip.Prefix{clientAddr}
	if clientAddrV6.IsValid() {
		assigned = append(assigned, clientAddrV6)
	}
	if err := sess.AssignAddresses(assigned); err != nil {
		log.Printf("TCP assign: %v", err)
		return
	}
	routes := []tcpRoute{{Start: h.route.Addr(), End: lastIPOfPrefix(h.route), IPProtocol: 0}}
	if h.routeV6.IsValid() {
		routes = append(routes, tcpRoute{Start: h.routeV6.Addr(), End: lastIPOfPrefix(h.routeV6), IPProtocol: 0})
	}
	if err := sess.AdvertiseRoutes(routes); err != nil {
		log.Printf("TCP advertise: %v", err)
		return
	}
	if err := relayClient(sess, h.tunDev, h.router, h.mtu, clientAddr, clientAddrV6); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		log.Printf("TCP session ended: %v", err)
	}
}

func serveTCP(ctx context.Context, ln net.Listener, base *tls.Config, h http.Handler) error {
	tlsConf := base.Clone()
	tlsConf.NextProtos = []string{"h2"}
	srv := &http.Server{
		Handler:           h,
		TLSConfig:         tlsConf,
		ReadHeaderTimeout: 0,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	err := srv.Serve(tls.NewListener(ln, tlsConf))
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
