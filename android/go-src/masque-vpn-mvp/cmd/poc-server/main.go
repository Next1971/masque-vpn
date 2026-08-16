// PoC/MVP MASQUE CONNECT-IP server based on quic-go/connect-ip-go (RFC 9484).
//
// Capabilities (stages C1→T1→mTLS→TUN→E1):
//   - real QUIC/HTTP3 + CONNECT-IP handshake (HTTP/3 stream hijacking);
//   - mTLS (-client-ca / tls.client_ca flag): mutual authentication;
//   - TUN forwarding (wireguard/tun): real conn↔masque0↔NAT traffic forwarding;
//   - IP pool (E1): dynamic address assignment to multiple clients;
//   - config.toml (E1): -config reads everything from TOML; without -config, flags are used.
//
// Multi-client routing: a single shared TUN device serves all clients. One
// reader goroutine reads the TUN and demultiplexes each inbound packet to the
// owning client connection by its destination IP address (see Router). Each
// client has its own conn→TUN goroutine. This prevents cross-delivery of return
// packets between simultaneous clients.
//
// Secrets (keys) are not stored in code—their paths are supplied through config/flags.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"
	"golang.zx2c4.com/wireguard/tun"
)

// tunOffset is reserved buffer space before packet data for wireguard/tun
// (on Linux, the device can use a virtio header). It matches
// device.MessageTransportHeaderSize from wireguard-go.
const tunOffset = 16

func main() {
	var (
		bind       = flag.String("bind", "0.0.0.0:4433", "UDP address to bind QUIC listener")
		certFile   = flag.String("cert", "cert/server.crt", "TLS certificate (PEM)")
		keyFile    = flag.String("key", "cert/server.key", "TLS private key (PEM)")
		serverName = flag.String("server-name", "YOUR_SERVER_HOST", "server name used in URI template")
		assignStr  = flag.String("assign", "10.8.0.2/32", "address prefix assigned to the client")
		routeStr   = flag.String("route", "0.0.0.0/0", "route advertised to the client")
		clientCA   = flag.String("client-ca", "", "CA to verify client certs (enables mTLS if set)")
		tunName    = flag.String("tun", "", "TUN interface name to forward packets (empty = log-only, no TUN)")
		tunAddr    = flag.String("tun-addr", "10.8.0.1/24", "CIDR address to assign on the TUN interface")
		mtu        = flag.Int("mtu", 1400, "MTU of the TUN interface")
		poolCIDR   = flag.String("pool", "", "CIDR pool for client addresses (empty = single -assign address)")
		configPath = flag.String("config", "", "path to config.toml (overrides individual flags)")
	)
	flag.Parse()

	var cfg serverConfig

	if *configPath != "" {
		// Configuration mode: read everything from TOML.
		c, err := LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		routePrefix, err := netip.ParsePrefix(c.Network.Route)
		if err != nil {
			log.Fatalf("bad network.route %q: %v", c.Network.Route, err)
		}
		cfg = serverConfig{
			bind: c.Server.Bind, certFile: c.TLS.Cert, keyFile: c.TLS.Key,
			serverName: c.Server.ServerName, clientCA: c.TLS.ClientCA,
			tunName: c.TUN.Name, tunAddr: c.Network.TunAddr, mtu: c.TUN.MTU,
			poolCIDR: c.Network.PoolCIDR, route: routePrefix,
		}
		log.Printf("loaded config from %s", *configPath)
	} else {
		// Flag mode (backward-compatible with tests).
		routePrefix, err := netip.ParsePrefix(*routeStr)
		if err != nil {
			log.Fatalf("bad -route %q: %v", *routeStr, err)
		}
		cfg = serverConfig{
			bind: *bind, certFile: *certFile, keyFile: *keyFile,
			serverName: *serverName, clientCA: *clientCA,
			tunName: *tunName, tunAddr: *tunAddr, mtu: *mtu,
			poolCIDR: *poolCIDR, route: routePrefix,
		}
		// If no pool is set, use the single address from -assign (legacy behavior).
		if *poolCIDR == "" {
			assignPrefix, err := netip.ParsePrefix(*assignStr)
			if err != nil {
				log.Fatalf("bad -assign %q: %v", *assignStr, err)
			}
			cfg.assign = assignPrefix
		}
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

type serverConfig struct {
	bind, certFile, keyFile, serverName, clientCA string
	tunName, tunAddr                              string
	mtu                                           int
	poolCIDR                                      string
	assign, route                                 netip.Prefix // assign is the fallback when no pool is set
}

// Router demultiplexes packets read from the single shared TUN device to the
// correct client connection, keyed by the client's assigned IP address.
//
// A single goroutine (see Run) reads the TUN device. For every inbound packet
// it extracts the destination IP and forwards the packet only to the connection
// that owns that address. Client connections register on connect and unregister
// on disconnect. This is what makes two or more simultaneous clients work: each
// return packet reaches exactly its owner instead of racing between clients.
type Router struct {
	mu      sync.RWMutex
	clients map[netip.Addr]*connectip.Conn
}

func NewRouter() *Router {
	return &Router{clients: make(map[netip.Addr]*connectip.Conn)}
}

// Add registers a client address → connection mapping.
func (r *Router) Add(addr netip.Addr, conn *connectip.Conn) {
	r.mu.Lock()
	r.clients[addr] = conn
	r.mu.Unlock()
}

// Remove drops a client mapping (only if the stored conn still matches).
func (r *Router) Remove(addr netip.Addr, conn *connectip.Conn) {
	r.mu.Lock()
	if r.clients[addr] == conn {
		delete(r.clients, addr)
	}
	r.mu.Unlock()
}

// lookup returns the connection owning dst, if any.
func (r *Router) lookup(dst netip.Addr) (*connectip.Conn, bool) {
	r.mu.RLock()
	conn, ok := r.clients[dst]
	r.mu.RUnlock()
	return conn, ok
}

// Run is the single TUN reader. It reads batches from the shared TUN device,
// finds the destination IP of each packet, and writes the packet to the owning
// client connection. It runs for the whole server lifetime.
func (r *Router) Run(dev tun.Device, mtu int) {
	batch := dev.BatchSize()
	if batch < 1 {
		batch = 1
	}
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunOffset+mtu+64)
	}
	for {
		k, err := dev.Read(bufs, sizes, tunOffset)
		if err != nil {
			log.Printf("router: tun read error: %v", err)
			return
		}
		for i := 0; i < k; i++ {
			pkt := bufs[i][tunOffset : tunOffset+sizes[i]]
			dst, ok := dstIP(pkt)
			if !ok {
				continue // not a parseable IPv4/IPv6 packet
			}
			conn, ok := r.lookup(dst)
			if !ok {
				// No client owns this address (stale/unknown) — drop.
				continue
			}
			if _, err := conn.WritePacket(pkt); err != nil {
				// The client connection is likely gone; the per-client
				// goroutine will unregister it. Drop and continue.
				continue
			}
		}
	}
}

func run(cfg serverConfig) error {
	bind, certFile, keyFile, serverName, clientCA := cfg.bind, cfg.certFile, cfg.keyFile, cfg.serverName, cfg.clientCA
	assign, route := cfg.assign, cfg.route

	// Open the TUN device (if -tun is set) and bring the interface up.
	var tunDev tun.Device
	if cfg.tunName != "" {
		dev, err := tun.CreateTUN(cfg.tunName, cfg.mtu)
		if err != nil {
			return fmt.Errorf("create TUN %q: %w", cfg.tunName, err)
		}
		tunDev = dev
		name, _ := dev.Name()
		if err := bringUpTUN(name, cfg.tunAddr); err != nil {
			dev.Close()
			return fmt.Errorf("bring up TUN: %w", err)
		}
		log.Printf("TUN %s up with %s (mtu %d)", name, cfg.tunAddr, cfg.mtu)
		defer dev.Close()
	} else {
		log.Printf("no -tun set: running in log-only mode (packets not forwarded)")
	}

	// Router: single reader of the shared TUN, demultiplexing to clients by dst IP.
	var router *Router
	if tunDev != nil {
		router = NewRouter()
		go router.Run(tunDev, cfg.mtu)
		log.Printf("router started: single TUN reader demultiplexing by destination IP")
	}

	// IP pool: assign addresses dynamically if pool_cidr is set.
	var pool *IPPool
	if cfg.poolCIDR != "" {
		serverTunAddr, err := netip.ParsePrefix(cfg.tunAddr)
		if err != nil {
			return fmt.Errorf("parse tun-addr %q: %w", cfg.tunAddr, err)
		}
		pool, err = NewIPPool(cfg.poolCIDR, serverTunAddr.Addr())
		if err != nil {
			return fmt.Errorf("build IP pool: %w", err)
		}
		log.Printf("IP pool %s ready (%d addresses available, server %s reserved)", cfg.poolCIDR, pool.Available(), serverTunAddr.Addr())
	}

	udpAddr, err := net.ResolveUDPAddr("udp", bind)
	if err != nil {
		return fmt.Errorf("resolve bind addr: %w", err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer udpConn.Close()

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load TLS keypair: %w", err)
	}

	tlsConf := &tls.Config{Certificates: []tls.Certificate{cert}}

	// mTLS: require and verify the client certificate if client-ca is set.
	if clientCA != "" {
		caPEM, err := os.ReadFile(clientCA)
		if err != nil {
			return fmt.Errorf("read client CA: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return fmt.Errorf("failed to parse client CA %q", clientCA)
		}
		tlsConf.ClientCAs = caPool
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		log.Printf("mTLS ENABLED: requiring client cert signed by %s", clientCA)
	}

	// URI template used by the client to access the proxy (RFC 9484 §3).
	template := uritemplate.MustNew(fmt.Sprintf("https://%s/vpn", serverName))

	ln, err := quic.ListenEarly(
		udpConn,
		http3.ConfigureTLSConfig(tlsConf),
		&quic.Config{EnableDatagrams: true},
	)
	if err != nil {
		return fmt.Errorf("QUIC listen: %w", err)
	}
	defer ln.Close()

	p := connectip.Proxy{}
	mux := http.NewServeMux()
	mux.HandleFunc("/vpn", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("CONNECT-IP request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		req, err := connectip.ParseRequest(r, template)
		if err != nil {
			var perr *connectip.RequestParseError
			if errors.As(err, &perr) {
				log.Printf("parse request error (HTTP %d): %v", perr.HTTPStatus, err)
				w.WriteHeader(perr.HTTPStatus)
				return
			}
			log.Printf("parse request error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Log the client certificate CN (if mTLS).
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			log.Printf("client authenticated via mTLS: CN=%s", r.TLS.PeerCertificates[0].Subject.CommonName)
		}

		conn, err := p.Proxy(w, req)
		if err != nil {
			log.Printf("proxy error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		log.Printf("CONNECT-IP session ESTABLISHED (HTTP/3 stream hijacked)")

		// Assign an address: from the pool when available, or the fixed fallback.
		clientAddr := assign
		if pool != nil {
			alloc, err := pool.Allocate()
			if err != nil {
				log.Printf("cannot allocate address: %v", err)
				conn.Close()
				return
			}
			clientAddr = alloc
			log.Printf("allocated %s from pool (%d left)", clientAddr, pool.Available())
			defer func() {
				pool.Release(alloc)
				log.Printf("released %s back to pool (%d available)", alloc, pool.Available())
			}()
		}

		if err := handleConn(conn, tunDev, router, cfg.mtu, clientAddr, route); err != nil {
			log.Printf("session ended: %v", err)
		}
	})

	srv := http3.Server{
		Handler:         mux,
		EnableDatagrams: true,
	}
	log.Printf("PoC MASQUE server (connect-ip-go) listening on %s (server-name=%s)", bind, serverName)
	if pool != nil {
		log.Printf("assigning client addresses from pool %s; advertising route %s", cfg.poolCIDR, route)
	} else {
		log.Printf("assigning fixed address %s; advertising route %s", assign, route)
	}
	go srv.ServeListener(ln)
	defer srv.Close()

	select {} // block forever
}

// handleConn assigns an address, advertises a route, then:
//   - if TUN is present, registers the client in the router and forwards its
//     conn→TUN direction (router handles TUN→conn for all clients);
//   - if TUN is absent (nil), uses the previous log-only mode.
func handleConn(conn *connectip.Conn, tunDev tun.Device, router *Router, mtu int, assign, route netip.Prefix) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.AssignAddresses(ctx, []netip.Prefix{assign}); err != nil {
		return fmt.Errorf("assign addresses: %w", err)
	}
	log.Printf("assigned %s to client", assign)

	lastIP := lastIPOfPrefix(route)
	if err := conn.AdvertiseRoute(ctx, []connectip.IPRoute{
		{StartIP: route.Addr(), EndIP: lastIP, IPProtocol: 0},
	}); err != nil {
		return fmt.Errorf("advertise route: %w", err)
	}
	log.Printf("advertised route %s - %s to client", route.Addr(), lastIP)

	if tunDev == nil {
		// Log-only mode (without TUN): read packets into the log.
		buf := make([]byte, 1500)
		for {
			n, err := conn.ReadPacket(buf)
			if err != nil {
				return fmt.Errorf("read packet: %w", err)
			}
			log.Printf("read %d bytes IP packet from client (ver=%d)", n, ipVersion(buf[:n]))
		}
	}

	// Register this client's address so the router delivers its return packets.
	clientIP := assign.Addr()
	router.Add(clientIP, conn)
	defer router.Remove(clientIP, conn)

	return forward(conn, tunDev, mtu)
}

// forward relays the client→TUN direction for a single client. The reverse
// direction (TUN→client) is handled centrally by the Router, which reads the
// shared TUN once and demultiplexes packets to the correct client by dst IP.
//
// When the client disconnects, conn.ReadPacket returns an error; we return it,
// which triggers the deferred router.Remove and pool.Release in the caller.
func forward(conn *connectip.Conn, dev tun.Device, mtu int) error {
	log.Printf("forwarding started (conn→TUN; TUN→conn via router)")
	buf := make([]byte, tunOffset+mtu+64)
	for {
		n, err := conn.ReadPacket(buf[tunOffset:])
		if err != nil {
			conn.Close()
			return fmt.Errorf("conn read: %w", err)
		}
		// wireguard/tun expects data with offset; pass the full slice with offset.
		if _, err := dev.Write([][]byte{buf[:tunOffset+n]}, tunOffset); err != nil {
			conn.Close()
			return fmt.Errorf("tun write: %w", err)
		}
	}
}

// dstIP extracts the destination IP address from a raw IPv4 or IPv6 packet.
// Returns ok=false if the packet is too short or not IPv4/IPv6.
func dstIP(pkt []byte) (netip.Addr, bool) {
	if len(pkt) < 1 {
		return netip.Addr{}, false
	}
	switch pkt[0] >> 4 {
	case 4:
		// IPv4 destination address is at bytes 16..19.
		if len(pkt) < 20 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte{pkt[16], pkt[17], pkt[18], pkt[19]}), true
	case 6:
		// IPv6 destination address is at bytes 24..39.
		if len(pkt) < 40 {
			return netip.Addr{}, false
		}
		var a [16]byte
		copy(a[:], pkt[24:40])
		return netip.AddrFrom16(a), true
	default:
		return netip.Addr{}, false
	}
}

// lastIPOfPrefix returns the final address in a prefix (the broadcast for an IPv4 range).
func lastIPOfPrefix(p netip.Prefix) netip.Addr {
	p = p.Masked()
	addr := p.Addr()
	if addr.Is4() {
		bytes := addr.As4()
		bits := p.Bits()
		for i := 0; i < 32-bits; i++ {
			byteIdx := 3 - i/8
			bitIdx := uint(i % 8)
			bytes[byteIdx] |= 1 << bitIdx
		}
		return netip.AddrFrom4(bytes)
	}
	bytes := addr.As16()
	bits := p.Bits()
	for i := 0; i < 128-bits; i++ {
		byteIdx := 15 - i/8
		bitIdx := uint(i % 8)
		bytes[byteIdx] |= 1 << bitIdx
	}
	return netip.AddrFrom16(bytes)
}

func ipVersion(b []byte) uint8 {
	if len(b) == 0 {
		return 0
	}
	return b[0] >> 4
}
