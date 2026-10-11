package clientcore

import (
	"bufio"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestMain(m *testing.M) {
	if !strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		v := os.Getenv("GODEBUG")
		if v != "" {
			v += ","
		}
		_ = os.Setenv("GODEBUG", v+"http2xconnect=1")
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Exit(ee.ExitCode())
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestTCPConnectAssignsAndEchoes(t *testing.T) {
	ca, caKey := mintCA(t)
	serverCert := mintLeaf(t, ca, caKey, "127.0.0.1", true)
	clientCert := mintLeaf(t, ca, caKey, "masque-client", false)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	got := make(chan []byte, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("handler %s %s protocol %q", r.Method, r.URL.Path, r.Header.Get(":protocol"))
			if r.Method != http.MethodConnect || r.Header.Get(":protocol") != "connect-ip" || r.URL.Path != "/vpn" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Capsule-Protocol", "?1")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			flusher.Flush()
			bw := bufio.NewWriter(w)
			assign, err := appendAddressAssign([]netip.Prefix{netip.MustParsePrefix("10.8.0.2/32")})
			if err != nil {
				return
			}
			route, err := appendRouteAdvertisement([]tcpRoute{{
				Start: netip.MustParseAddr("0.0.0.0"),
				End:   netip.MustParseAddr("255.255.255.255"),
			}})
			if err != nil {
				return
			}
			if _, err := bw.Write(assign); err != nil {
				return
			}
			if _, err := bw.Write(route); err != nil {
				return
			}
			if err := bw.Flush(); err != nil {
				return
			}
			flusher.Flush()
			br := bufio.NewReader(r.Body)
			typ, val, err := readCapsule(br)
			if err != nil || typ != capsuleDatagram {
				return
			}
			pkt, err := parseDatagram(val)
			if err != nil {
				return
			}
			got <- append([]byte(nil), pkt...)
			echo := appendDatagram([]byte("pong"))
			if _, err := bw.Write(echo); err != nil {
				return
			}
			if err := bw.Flush(); err != nil {
				return
			}
			flusher.Flush()
			_, _ = io.Copy(io.Discard, r.Body)
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientCAs:    pool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		},
	}
	if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := srv.ServeTLS(ln, "", "")
		t.Logf("serve exit: %v", err)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	certPath := filepath.Join(dir, "client.pem")
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(clientCert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := connectTCP(ctx, &Profile{
		Server:     ln.Addr().String(),
		ServerName: "127.0.0.1",
		CA:         caPath,
		Cert:       certPath,
		Key:        keyPath,
		Transport:  "tcp",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if len(sess.AssignedPrefixes) != 1 || sess.AssignedPrefixes[0].String() != "10.8.0.2/32" {
		t.Fatalf("assigned %v", sess.AssignedPrefixes)
	}
	if sess.UDPFd() <= 0 {
		t.Fatal("tcp socket fd missing")
	}
	if _, err := sess.ipconn.WritePacket([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	select {
	case pkt := <-got:
		if string(pkt) != "ping" {
			t.Fatalf("server got %q", pkt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not receive the packet")
	}
	buf := make([]byte, 64)
	n, err := readPacketTimeout(sess.ipconn, buf, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "pong" {
		t.Fatalf("echo %q", buf[:n])
	}
}

func mintCA(t *testing.T) (*x509.Certificate, crypto.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "masque-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func mintLeaf(t *testing.T, ca *x509.Certificate, caKey crypto.PrivateKey, cn string, server bool) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	if server {
		if ip, err := netip.ParseAddr(cn); err == nil {
			tmpl.IPAddresses = []net.IP{net.IP(ip.AsSlice())}
		} else {
			tmpl.DNSNames = []string{cn}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, key.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}
}
