package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	clientCAs      *x509.CertPool
	pool, poolV6   *IPPool
	assign         netip.Prefix
	route, routeV6 netip.Prefix
	tunDev         tun.Device
	router         *Router
	mtu            int
	live           *sessionIndex
}

func (h tcpConnect) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cn, ok := h.allowTunnel(r)
	if !ok || !tunnelRequest(r) {
		if tunnelRequest(r) {
			log.Printf("TCP site instead of tunnel from %s", r.RemoteAddr)
		}
		serveCover(w)
		return
	}
	log.Printf("TCP client authenticated via mTLS: CN=%s", cn)

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

// tunnelRequest is the MASQUE HTTP/2 CONNECT-IP call. Anything else is a
// site visit or a probe and is answered with the cover page.
func tunnelRequest(r *http.Request) bool {
	return r.Method == http.MethodConnect &&
		r.Header.Get(":protocol") == h2ConnectProtocol &&
		r.URL.Path == h2RequestPath &&
		r.Header.Get(h2CapsuleHeader) == h2CapsuleValue
}

// allowTunnel reports whether this connection may open a VPN session.
// With a client CA, the certificate is checked here so a missing or foreign
// certificate can still receive the site. Without a CA, TLS policy stands.
func (h tcpConnect) allowTunnel(r *http.Request) (string, bool) {
	if h.clientCAs != nil {
		cn, ok := verifyClientCert(r, h.clientCAs)
		if !ok {
			return "", false
		}
		if _, deny := h.blocked[cn]; deny {
			log.Printf("rejected blocked CN=%s", cn)
			return "", false
		}
		return cn, true
	}
	cn := ""
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		cn = r.TLS.PeerCertificates[0].Subject.CommonName
	}
	if cn != "" {
		if _, deny := h.blocked[cn]; deny {
			log.Printf("rejected blocked CN=%s", cn)
			return "", false
		}
	}
	return cn, true
}

func verifyClientCert(r *http.Request, roots *x509.CertPool) (string, bool) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", false
	}
	leaf := r.TLS.PeerCertificates[0]
	inter := x509.NewCertPool()
	for _, cert := range r.TLS.PeerCertificates[1:] {
		inter.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return "", false
	}
	return leaf.Subject.CommonName, true
}

func serveTCP(ctx context.Context, ln net.Listener, base *tls.Config, h http.Handler) error {
	tlsConf := base.Clone()
	tlsConf.NextProtos = []string{"h2", "http/1.1"}
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
