package vpssetup

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)

// ProbeUDPListener sends a QUIC ClientHello to host:udpPort after the MASQUE
// process should already be listening. A timeout means the datagram did not
// come back (cloud SG, wrong IP, or the service is down). Any cryptographic
// or protocol error after a reply counts as OK — mTLS will reject this probe.
func ProbeUDPListener(ctx context.Context, host string, udpPort int) error {
	if err := ValidateHost(host); err != nil {
		return err
	}
	if err := ValidateUDPPort(udpPort); err != nil {
		return err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(udpPort))
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3"},
		ServerName:         host,
	}
	qconf := &quic.Config{
		HandshakeIdleTimeout: 8 * time.Second,
		MaxIdleTimeout:       8 * time.Second,
	}
	conn, err := quic.DialAddr(ctx, addr, tlsConf, qconf)
	if conn != nil {
		_ = conn.CloseWithError(0, "")
	}
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("no QUIC reply from %s (open UDP %d in the cloud firewall if the service is running)", addr, udpPort)
	}
	if isTimeout(err) {
		return fmt.Errorf("no QUIC reply from %s (open UDP %d in the cloud firewall if the service is running)", addr, udpPort)
	}
	// Handshake failure / mTLS / VERSION_NEGOTIATION: the port answered.
	return nil
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") || strings.Contains(s, "no recent network activity")
}
