package vpssetup

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var hostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateHost is a DNS name or dotted IPv4 used in SSH, certificates, and remote shell env.
func ValidateHost(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("host is empty")
	}
	if ip := net.ParseIP(s); ip != nil {
		if ip.To4() == nil {
			return fmt.Errorf("IPv6 hosts are not supported in this installer")
		}
		return nil
	}
	if !hostRe.MatchString(s) || strings.Contains(s, "..") {
		return fmt.Errorf("invalid host %q", s)
	}
	return nil
}

// ValidateUDPPort checks a user-selected listen port.
func ValidateUDPPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be 1–65535")
	}
	if p == 22 {
		return fmt.Errorf("port 22 is reserved for SSH")
	}
	return nil
}
