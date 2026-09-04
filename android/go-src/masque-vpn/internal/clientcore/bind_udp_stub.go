//go:build !darwin

package clientcore

import "net"

func bindUDPToInterface(_ *net.UDPConn, _ string) error { return nil }
