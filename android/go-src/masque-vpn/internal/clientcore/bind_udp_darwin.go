//go:build darwin

package clientcore

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func bindUDPToInterface(conn *net.UDPConn, name string) error {
	if name == "" {
		return nil
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return fmt.Errorf("interface: %w", err)
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var soerr error
	if err := raw.Control(func(fd uintptr) {
		soerr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, ifi.Index)
	}); err != nil {
		return err
	}
	return soerr
}
