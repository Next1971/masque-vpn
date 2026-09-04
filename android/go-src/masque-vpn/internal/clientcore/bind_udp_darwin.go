//go:build darwin

package clientcore

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// bindUDPToInterface pins a UDP socket to a named interface (IP_BOUND_IF).
// It must never panic: a Network Extension will kill the Packet Tunnel on
// unrecovered Go panics. Callers must treat a returned error as non-fatal.
func bindUDPToInterface(conn *net.UDPConn, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bind UDP to %q panic: %v", name, r)
		}
	}()
	if name == "" || conn == nil {
		return nil
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return fmt.Errorf("interface: %w", err)
	}
	if ifi == nil || ifi.Index <= 0 {
		return fmt.Errorf("interface %q has unusable index", name)
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var soerr error
	if err := raw.Control(func(fd uintptr) {
		if fd == 0 {
			soerr = fmt.Errorf("invalid udp fd")
			return
		}
		soerr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, ifi.Index)
	}); err != nil {
		return err
	}
	return soerr
}
