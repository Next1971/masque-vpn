// PoC MASQUE CONNECT-IP client based on quic-go/connect-ip-go.
//
// Stage 1 (C1/T1): WITHOUT TUN and WITHOUT mTLS.
// Runs from a sandbox (no /dev/net/tun, no root) against a PoC server on a VPS.
// The goal is to prove that the CONNECT-IP handshake succeeds: Dial returns 200,
// the client receives an assigned prefix (LocalPrefixes) and routes (Routes),
// then sends several handcrafted IP packets into the tunnel (WritePacket) and
// attempts to read a reply. No TUN is created—this tests the protocol only.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"
)

func main() {
	var (
		proxyAddr  = flag.String("proxy", "YOUR_SERVER_HOST:4433", "UDP host:port of the MASQUE proxy")
		serverName = flag.String("server-name", "YOUR_SERVER_HOST", "TLS SNI / URI template host")
		timeout    = flag.Duration("timeout", 10*time.Second, "overall timeout")
		caFile     = flag.String("ca", "", "CA to verify the server certificate (required)")
		certFile   = flag.String("cert", "", "client certificate (PEM) for mTLS")
		keyFile    = flag.String("key", "", "client private key (PEM) for mTLS")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := run(ctx, *proxyAddr, *serverName, *caFile, *certFile, *keyFile); err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	log.Printf("PoC OK: CONNECT-IP handshake completed end-to-end")
}

func run(ctx context.Context, proxyAddr, serverName, caFile, certFile, keyFile string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", proxyAddr)
	if err != nil {
		return fmt.Errorf("resolve proxy addr: %w", err)
	}
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer udpConn.Close()

	tlsConf := &tls.Config{
		ServerName: serverName,
		NextProtos: []string{http3.NextProtoH3},
	}

	if caFile == "" {
		return fmt.Errorf("server CA is required; pass -ca")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA %q", caFile)
	}
	tlsConf.RootCAs = pool
	log.Printf("verifying server cert against CA %s", caFile)

	// Client certificate for mTLS, if set.
	if certFile != "" && keyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("load client keypair: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{clientCert}
		log.Printf("presenting client cert %s for mTLS", certFile)
	}

	conn, err := quic.Dial(ctx, udpConn, udpAddr,
		tlsConf,
		&quic.Config{
			EnableDatagrams:   true,
			InitialPacketSize: 1350,
			KeepAlivePeriod:   15 * time.Second,
			MaxIdleTimeout:    3 * time.Minute,
		},
	)
	if err != nil {
		return fmt.Errorf("QUIC dial: %w", err)
	}
	log.Printf("QUIC connection established to %s", proxyAddr)

	tr := &http3.Transport{EnableDatagrams: true}
	hconn := tr.NewClientConn(conn)

	template := uritemplate.MustNew(fmt.Sprintf("https://%s/vpn", serverName))
	ipconn, rsp, err := connectip.Dial(ctx, hconn, template)
	if err != nil {
		return fmt.Errorf("connect-ip dial: %w", err)
	}
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", rsp.StatusCode)
	}
	log.Printf("CONNECT-IP dial OK, HTTP status %d", rsp.StatusCode)
	defer ipconn.Close()

	// Read assigned addresses and routes (evidence of capsule exchange).
	prefixes, err := ipconn.LocalPrefixes(ctx)
	if err != nil {
		return fmt.Errorf("get local prefixes: %w", err)
	}
	log.Printf("server assigned prefixes: %v", prefixes)

	routes, err := ipconn.Routes(ctx)
	if err != nil {
		return fmt.Errorf("get routes: %w", err)
	}
	for _, r := range routes {
		log.Printf("server advertised route: %s - %s (proto %d)", r.StartIP, r.EndIP, r.IPProtocol)
	}

	if len(prefixes) == 0 {
		return fmt.Errorf("no prefixes assigned by server")
	}

	// Send a few test IP packets into the tunnel (WritePacket).
	src := prefixes[0].Addr()
	dst := netip.MustParseAddr("1.1.1.1")
	for i := 0; i < 3; i++ {
		pkt := buildICMPEchoRequest(src, dst, uint16(i))
		icmp, err := ipconn.WritePacket(pkt)
		if err != nil {
			return fmt.Errorf("write packet %d: %w", i, err)
		}
		log.Printf("wrote test IP packet %d (%d bytes), icmp-return=%d bytes", i, len(pkt), len(icmp))
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("handshake + address/route exchange + packet write all succeeded")

	// Read reply packets from the tunnel (evidence that traffic returns
	// TUN→NAT→Internet→back). Wait for an ICMP Echo Reply from 1.1.1.1.
	log.Printf("waiting for reply packets (up to 3s)...")
	replyDeadline := time.Now().Add(3 * time.Second)
	gotReply := false
	buf := make([]byte, 1500)
	for time.Now().Before(replyDeadline) {
		readCtx, cancel := context.WithDeadline(context.Background(), replyDeadline)
		n, err := readPacketCtx(readCtx, ipconn, buf)
		cancel()
		if err != nil {
			break
		}
		if n >= 20 {
			proto := buf[9]
			srcIP := netip.AddrFrom4([4]byte{buf[12], buf[13], buf[14], buf[15]})
			icmpType := -1
			if proto == 1 && n >= 20+1 {
				icmpType = int(buf[20])
			}
			log.Printf("REPLY: %d bytes from %s (proto=%d icmp-type=%d)", n, srcIP, proto, icmpType)
			if proto == 1 && icmpType == 0 {
				log.Printf("✅ ICMP Echo Reply received — end-to-end tunnel WORKS")
				gotReply = true
				break
			}
		}
	}
	if !gotReply {
		log.Printf("⚠ no ICMP reply within timeout (tunnel data-plane may need real client, or ICMP filtered)")
	}
	return nil
}

// readPacketCtx wraps ReadPacket with a timeout through a goroutine.
func readPacketCtx(ctx context.Context, conn *connectip.Conn, buf []byte) (int, error) {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		n, err := conn.ReadPacket(buf)
		ch <- res{n, err}
	}()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case r := <-ch:
		return r.n, r.err
	}
}

// buildICMPEchoRequest builds a minimal IPv4+ICMP Echo Request packet.
func buildICMPEchoRequest(src, dst netip.Addr, seq uint16) []byte {
	const ipHdrLen = 20
	const icmpLen = 8
	total := ipHdrLen + icmpLen
	b := make([]byte, total)

	// IPv4 header
	b[0] = 0x45 // version 4, IHL 5
	b[1] = 0x00
	b[2] = byte(total >> 8)
	b[3] = byte(total)
	b[4], b[5] = 0x00, 0x00 // id
	b[6], b[7] = 0x40, 0x00 // flags: DF
	b[8] = 64               // TTL
	b[9] = 1                // protocol ICMP
	s4 := src.As4()
	d4 := dst.As4()
	copy(b[12:16], s4[:])
	copy(b[16:20], d4[:])
	putChecksum(b[0:ipHdrLen], 10) // IP header checksum at offset 10

	// ICMP echo request
	icmp := b[ipHdrLen:]
	icmp[0] = 8 // type: echo request
	icmp[1] = 0 // code
	icmp[4], icmp[5] = 0x12, 0x34
	icmp[6] = byte(seq >> 8)
	icmp[7] = byte(seq)
	putChecksum(icmp, 2) // ICMP checksum at offset 2

	return b
}

// putChecksum calculates the Internet checksum for a buffer and stores it at offset.
func putChecksum(b []byte, offset int) {
	b[offset] = 0
	b[offset+1] = 0
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	cs := ^uint16(sum)
	b[offset] = byte(cs >> 8)
	b[offset+1] = byte(cs)
}
