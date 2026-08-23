//go:build !windows

package main

import "fmt"

func runServiceCLI(status, connect, disconnect bool, importPath string) error {
	return fmt.Errorf("the MASQUE Windows service CLI is only available on Windows")
}
