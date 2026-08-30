//go:build linux

// iface_linux.go brings up a TUN interface on Linux using the `ip` utility.
// Isolated by a build tag so the shared core remains cross-platform:
// Windows/Android use their own interface configuration (wintun / VpnService).
package main

import (
	"fmt"
	"os/exec"
)

// bringUpTUN assigns one or more CIDR addresses to the TUN and brings it up.
func bringUpTUN(name string, addrs ...string) error {
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		if out, err := exec.Command("ip", "addr", "add", addr, "dev", name).CombinedOutput(); err != nil {
			return fmt.Errorf("ip addr add %s: %w: %s", addr, err, out)
		}
	}
	if out, err := exec.Command("ip", "link", "set", name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set up: %w: %s", err, out)
	}
	return nil
}
