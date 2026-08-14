package clientcore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sync"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"
	"golang.zx2c4.com/wireguard/tun"
)

// tunOffset is reserved buffer space before packet data for wireguard/tun
// (device.MessageTransportHeaderSize from wireguard-go). It must match
// the server value.
const tunOffset = 16

// Verbose enables detailed diagnostic logs (per-packet tracing of
// conn→TUN, TTL normalization, and so on). It is disabled by default because
// these logs are noisy in production. Wrappers set it from the -verbose flag.
var Verbose bool

// vlog prints diagnostic messages only when Verbose=true.
func vlog(format string, args ...any) {
	if Verbose {
		log.Printf(format, args...)
	}
}

// Session is an active VPN connection. It owns QUIC/UDP resources and TUN
// and can close gracefully. It is created by Connect.
type Session struct {
	udpConn *net.UDPConn
	qconn   *quic.Conn
	ipconn  *connectip.Conn
	dev     tun.Device

	// AssignedPrefixes are addresses assigned to the client by the server (for
	// TUN and route configuration by the platform wrapper).
	AssignedPrefixes []netip.Prefix
	// Routes are routes advertised by the server (usually 0.0.0.0/0).
	Routes []connectip.IPRoute

	closeOnce sync.Once
	done      chan struct{}
}

// buildTLSConfig builds tls.Config from the profile: server certificate verification
// by CA (required when a CA is set) plus a client certificate for mTLS.
func buildTLSConfig(p *Profile) (*tls.Config, error) {
	tlsConf := &tls.Config{
		ServerName: p.ServerName,
		NextProtos: []string{http3.NextProtoH3},
	}

	if p.CA != "" {
		caPEM, err := os.ReadFile(p.CA)
		if err != nil {
			return nil, fmt.Errorf("read CA %q: %w", p.CA, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA %q", p.CA)
		}
		tlsConf.RootCAs = pool
	} else {
		// Without a CA there is nothing to verify the server against, so
		// verification must be skipped regardless of the Insecure flag.
		tlsConf.InsecureSkipVerify = true
	}

	// Optional "disable certificate verification" toggle ([tls].insecure in the
	// profile). INSECURE: it disables authentication of the server, so it should
	// only be used to troubleshoot certificate problems.
	if p.Insecure {
		tlsConf.InsecureSkipVerify = true
	}

	if p.Cert != "" && p.Key != "" {
		clientCert, err := tls.LoadX509KeyPair(p.Cert, p.Key)
		if err != nil {
			return nil, fmt.Errorf("load client keypair: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{clientCert}
	}
	return tlsConf, nil
}

// Connect establishes a MASQUE CONNECT-IP session from the profile.
// dev is a ready TUN interface created externally by the platform wrapper
// (the core does not create TUN or modify routes). After Connect succeeds,
// the caller configures the address/routes from s.AssignedPrefixes/s.Routes,
// then starts s.Run(ctx).
func Connect(ctx context.Context, p *Profile, dev tun.Device) (*Session, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", p.Server)
	if err != nil {
		return nil, fmt.Errorf("resolve server %q: %w", p.Server, err)
	}
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}

	tlsConf, err := buildTLSConfig(p)
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	qconn, err := quic.Dial(ctx, udpConn, udpAddr, tlsConf, &quic.Config{
		EnableDatagrams:   true,
		InitialPacketSize: 1350,
	})
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("QUIC dial: %w", err)
	}
	log.Printf("QUIC connection established to %s", p.Server)

	tr := &http3.Transport{EnableDatagrams: true}
	hconn := tr.NewClientConn(qconn)

	template := uritemplate.MustNew(fmt.Sprintf("https://%s/vpn", p.ServerName))
	ipconn, rsp, err := connectip.Dial(ctx, hconn, template)
	if err != nil {
		qconn.CloseWithError(0, "")
		udpConn.Close()
		return nil, fmt.Errorf("connect-ip dial: %w", err)
	}
	if rsp.StatusCode != http.StatusOK {
		ipconn.Close()
		qconn.CloseWithError(0, "")
		udpConn.Close()
		return nil, fmt.Errorf("unexpected CONNECT-IP status: %d", rsp.StatusCode)
	}
	log.Printf("CONNECT-IP session established (HTTP %d)", rsp.StatusCode)

	prefixes, err := ipconn.LocalPrefixes(ctx)
	if err != nil {
		ipconn.Close()
		qconn.CloseWithError(0, "")
		udpConn.Close()
		return nil, fmt.Errorf("get local prefixes: %w", err)
	}
	if len(prefixes) == 0 {
		ipconn.Close()
		qconn.CloseWithError(0, "")
		udpConn.Close()
		return nil, fmt.Errorf("server assigned no prefixes")
	}
	log.Printf("server assigned prefixes: %v", prefixes)

	routes, err := ipconn.Routes(ctx)
	if err != nil {
		ipconn.Close()
		qconn.CloseWithError(0, "")
		udpConn.Close()
		return nil, fmt.Errorf("get routes: %w", err)
	}
	for _, r := range routes {
		log.Printf("server advertised route: %s - %s (proto %d)", r.StartIP, r.EndIP, r.IPProtocol)
	}

	return &Session{
		udpConn:          udpConn,
		qconn:            qconn,
		ipconn:           ipconn,
		dev:              dev,
		AssignedPrefixes: prefixes,
		Routes:           routes,
		done:             make(chan struct{}),
	}, nil
}

// Run starts bidirectional conn↔TUN forwarding and blocks until
// completion (an error on either side, s.Close(), or context cancellation).
// It returns the first termination reason.
func (s *Session) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	mtu, err := s.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = 1400
	}

	// conn → TUN: write packets from the server (Internet replies) to TUN,
	// where the OS reads them and delivers them to applications.
	go func() {
		buf := make([]byte, tunOffset+mtu+64)
		var inCount int // diagnostics: packets received from conn into TUN
		for {
			n, err := s.ipconn.ReadPacket(buf[tunOffset:])
			if err != nil {
				errCh <- fmt.Errorf("conn read: %w", err)
				return
			}
			inCount++
			if Verbose && inCount <= 6 {
				vlog("conn→TUN packet #%d: %s (%d bytes)", inCount, describePkt(buf[tunOffset:tunOffset+n]), n)
			}
			if _, err := s.dev.Write([][]byte{buf[:tunOffset+n]}, tunOffset); err != nil {
				errCh <- fmt.Errorf("tun write: %w", err)
				return
			}
		}
	}()

	// TUN → conn: send application packets (from TUN) to the server.
	go func() {
		batch := s.dev.BatchSize()
		if batch < 1 {
			batch = 1
		}
		bufs := make([][]byte, batch)
		sizes := make([]int, batch)
		for i := range bufs {
			bufs[i] = make([]byte, tunOffset+mtu+64)
		}
		var fixedCount int // packets with raised TTL (diagnostics)
		for {
			k, err := s.dev.Read(bufs, sizes, tunOffset)
			if err != nil {
				errCh <- fmt.Errorf("tun read: %w", err)
				return
			}
			select {
			case <-s.done:
				return
			default:
			}
			for i := 0; i < k; i++ {
				pkt := bufs[i][tunOffset : tunOffset+sizes[i]]
				// Raise an excessively low TTL/Hop Limit; otherwise connect-ip
				// drops the packet ("Hop Limit too small"). This shared fix applies to all
				// platforms (especially Windows routing into TUN).
				if orig, fixed := normalizeTTL(pkt); fixed {
					fixedCount++
					if fixedCount <= 3 {
						vlog("raised low TTL/HopLimit %d→%d on outgoing packet (%d bytes)", orig, fixTTL, len(pkt))
					}
				}
				if _, err := s.ipconn.WritePacket(pkt); err != nil {
					errCh <- fmt.Errorf("conn write: %w", err)
					return
				}
			}
		}
	}()

	log.Printf("forwarding started (conn↔TUN)")

	var runErr error
	select {
	case runErr = <-errCh:
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	s.Close()
	return runErr
}

// Close terminates the session gracefully: it closes CONNECT-IP (the server immediately
// returns the address to the pool), then QUIC and UDP. TUN is NOT closed here;
// the platform wrapper manages its lifecycle (it created it and
// closes it, together with route rollback).
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		if s.ipconn != nil {
			s.ipconn.Close() // sends CONNECT-IP close → server calls Release
		}
		if s.qconn != nil {
			s.qconn.CloseWithError(0, "client shutdown")
		}
		if s.udpConn != nil {
			s.udpConn.Close()
		}
		log.Printf("session closed gracefully")
	})
	return nil
}
