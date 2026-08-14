//go:build linux

// iface_linux.go brings up a TUN interface on Linux using the `ip` utility.
// Isolated by a build tag so the shared core remains cross-platform:
// Windows/Android use their own interface configuration (wintun / VpnService).
package main

import (
	"fmt"
	"os/exec"
)

// bringUpTUN assigns an address to the TUN interface and brings it up.
// addr is in CIDR format, e.g. "10.8.0.1/24".
func bringUpTUN(name, addr string) error {
	// ip addr add <addr> dev <name>
	if out, err := exec.Command("ip", "addr", "add", addr, "dev", name).CombinedOutput(); err != nil {
		return fmt.Errorf("ip addr add: %w: %s", err, out)
	}
	// ip link set <name> up
	if out, err := exec.Command("ip", "link", "set", name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set up: %w: %s", err, out)
	}
	return nil
}
