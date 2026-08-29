package vpssetup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ProbeUDPListener checks that something on host:udpPort answers a UDP
// datagram after the MASQUE listener should be up. A timeout means the packet
// did not come back (cloud SG, wrong IP, or the service is down). Any reply
// counts as OK — this is not a TLS trust check.
func ProbeUDPListener(ctx context.Context, host string, udpPort int) error {
	if err := ValidateHost(host); err != nil {
		return err
	}
	if err := ValidateUDPPort(udpPort); err != nil {
		return err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(udpPort))
	var d net.Dialer
	c, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return fmt.Errorf("udp dial %s: %w", addr, err)
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	} else {
		_ = c.SetDeadline(time.Now().Add(8 * time.Second))
	}
	// One datagram is enough for quic-go to send Version Negotiation or a drop
	// response; we only need to see that UDP is not black-holed.
	if _, err := c.Write([]byte{0xc0, 0x00, 0x00, 0x00, 0x01}); err != nil {
		return fmt.Errorf("udp write %s: %w", addr, err)
	}
	buf := make([]byte, 1500)
	n, err := c.Read(buf)
	if err == nil && n > 0 {
		return nil
	}
	if ctx.Err() != nil || isTimeout(err) {
		return fmt.Errorf("no UDP reply from %s (open UDP %d in the cloud firewall if the service is running)", addr, udpPort)
	}
	if err != nil {
		return fmt.Errorf("udp read %s: %w", addr, err)
	}
	return fmt.Errorf("no UDP reply from %s (open UDP %d in the cloud firewall if the service is running)", addr, udpPort)
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
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") || strings.Contains(s, "i/o timeout")
}
