//go:build !linux

package main

import "fmt"

func bringUpTUN(name string, addrs ...string) error {
	return fmt.Errorf("TUN bring-up is only implemented on Linux (iface %s addrs %v)", name, addrs)
}
