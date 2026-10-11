package main

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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
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

func TestAddressAssignRoundTrip(t *testing.T) {
	want := netip.MustParsePrefix("10.8.0.2/32")
	frame, err := appendAddressAssign([]netip.Prefix{want})
	if err != nil {
		t.Fatal(err)
	}
	typ, val, err := readCapsule(bufio.NewReader(strings.NewReader(string(frame))))
	if err != nil {
		t.Fatal(err)
	}
	if typ != capsuleAddressAssign {
		t.Fatalf("type %d", typ)
	}
	got, err := parseAddressAssign(val)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("assign %v", got)
	}
}

func TestTCPConnectAssignsAddress(t *testing.T) {
	ca, caKey := mintCA(t)
	serverCert := mintLeaf(t, ca, caKey, "masque.test", true)
	clientCert := mintLeaf(t, ca, caKey, "masque-client-1", false)
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		errc <- serveTCP(ctx, ln, &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientCAs:    pool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}, tcpConnect{
			blocked: map[string]struct{}{},
			assign:  netip.MustParsePrefix("10.8.0.2/32"),
			route:   netip.MustParsePrefix("0.0.0.0/0"),
			live:    &sessionIndex{byCN: make(map[string]clientLink)},
		})
	}()

	tr := &http2.Transport{TLSClientConfig: &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
		ServerName:   "masque.test",
		NextProtos:   []string{"h2"},
	}}
	req, err := http.NewRequest(http.MethodConnect, "https://"+ln.Addr().String()+"/vpn", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	pr, pw := io.Pipe()
	req.Body = pr
	req.ContentLength = -1
	req.Header[":protocol"] = []string{h2ConnectProtocol}
	req.Header.Set(h2CapsuleHeader, h2CapsuleValue)

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	defer pw.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	typ, val, err := readCapsule(br)
	if err != nil {
		t.Fatal(err)
	}
	if typ != capsuleAddressAssign {
		t.Fatalf("type %d", typ)
	}
	got, err := parseAddressAssign(val)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != netip.MustParsePrefix("10.8.0.2/32") {
		t.Fatalf("assign %v", got)
	}
	typ, val, err = readCapsule(br)
	if err != nil {
		t.Fatal(err)
	}
	if typ != capsuleRouteAdvertisement {
		t.Fatalf("route type %d", typ)
	}
	routes, err := parseRouteAdvertisement(val)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].Start != netip.MustParseAddr("0.0.0.0") || routes[0].End != netip.MustParseAddr("255.255.255.255") {
		t.Fatalf("route %+v", routes)
	}

	bare := &http2.Transport{TLSClientConfig: &tls.Config{
		RootCAs:    pool,
		ServerName: "masque.test",
		NextProtos: []string{"h2"},
	}}
	badReq, err := http.NewRequest(http.MethodConnect, "https://"+ln.Addr().String()+"/vpn", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	badReq.Header[":protocol"] = []string{h2ConnectProtocol}
	badReq.Header.Set(h2CapsuleHeader, h2CapsuleValue)
	if _, err := bare.RoundTrip(badReq); err == nil {
		t.Fatal("client without a certificate was accepted")
	}
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tcp server did not stop")
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
		tmpl.DNSNames = []string{cn}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, key.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}
}
