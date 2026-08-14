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

// tunOffset is the buffer space before packet data for wireguard/tun
// (device.MessageTransportHeaderSize from wireguard-go). It must match
// the server-side value.
const tunOffset = 16

// Verbose enables detailed diagnostic logs (per-packet tracing
// conn→TUN, TTL normalization, etc.). Disabled by default because
// these logs are too noisy for production. Wrappers set it via -verbose.
var Verbose bool

// vlog prints a diagnostic message only when Verbose=true.
func vlog(format string, args ...any) {
	if Verbose {
		log.Printf(format, args...)
	}
}

// Session is an active VPN connection. It keeps QUIC/UDP resources and TUN,
// and supports graceful shutdown. Created via Connect.
type Session struct {
	udpConn *net.UDPConn
	qconn   *quic.Conn
	ipconn  *connectip.Conn
	dev     tun.Device

	// AssignedPrefixes are the addresses assigned by the server to the client
	// (used by the platform wrapper to configure TUN and routes).
	AssignedPrefixes []netip.Prefix
	// Routes are the routes advertised by the server (usually 0.0.0.0/0).
	Routes []connectip.IPRoute

	closeOnce sync.Once
	done      chan struct{}
}

// buildTLSConfig builds tls.Config from the profile: server certificate
// verification using CA (mandatory if CA is set) + client certificate for mTLS.
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

	// Optional "disable certificate verification" toggle (checkbox in the UI /
	// -insecure flag / [tls].insecure in the profile). Some setups fail the TLS
	// handshake on server certificate validation (self-signed / hostname
	// mismatch); enabling this lets the client connect anyway. INSECURE: it
	// disables authentication of the server, so use it only for troubleshooting.
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

// Connect establishes a MASQUE CONNECT-IP session using the profile.
// dev is a ready TUN interface created externally by the platform wrapper
// (the core does not create TUN or modify routes). After a successful Connect,
// the caller configures address/routes using s.AssignedPrefixes/s.Routes,
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
// completion (an error on either side, s.Close(), or ctx cancellation).
// Returns the first stop reason.
func (s *Session) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	mtu, err := s.dev.MTU()
	if err != nil || mtu <= 0 {
		mtu = 1400
	}

	// conn → TUN: write packets from the server (responses from the internet)
	// into TUN, where the OS reads them and delivers them to applications.
	go func() {
		buf := make([]byte, tunOffset+mtu+64)
		var inCount int // diagnostics: how many packets came from conn into TUN
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

	// TUN → conn: send packets from applications (from TUN) to the server.
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
		var fixedCount int // how many packets had TTL raised (diagnostics)
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
				// Raise too-small TTL/Hop Limit, otherwise connect-ip
				// will drop the packet ("Hop Limit too small"). A general fix for all
				// platforms (especially relevant for Windows routing into TUN).
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

// Close gracefully terminates the session: closes CONNECT-IP first (so the server
// immediately releases the address back to the pool), then QUIC and UDP. TUN is NOT
// closed here — its lifecycle is managed by the platform wrapper (it created it, so it
// also closes it, together with route rollback).
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		if s.ipconn != nil {
			s.ipconn.Close() // sends CONNECT-IP close → server performs Release
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