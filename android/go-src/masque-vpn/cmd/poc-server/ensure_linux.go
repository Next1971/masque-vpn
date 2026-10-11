//go:build linux

package main

import (
	"log"
	"os"
	"strings"
	"syscall"
)

// ensureHTTP2Connect re-execs this process with GODEBUG=http2xconnect=1.
// The standard library reads that variable in init and otherwise rejects
// HTTP/2 extended CONNECT. syscall.Exec keeps the same pid, so systemd
// does not treat the restart as a service exit. UDP keeps working if exec fails.
func ensureHTTP2Connect() {
	if strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		return
	}
	v := os.Getenv("GODEBUG")
	if v != "" {
		v += ","
	}
	if err := os.Setenv("GODEBUG", v+"http2xconnect=1"); err != nil {
		log.Printf("TCP CONNECT-IP disabled: %v", err)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		log.Printf("TCP CONNECT-IP disabled: %v", err)
		return
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		log.Printf("TCP CONNECT-IP disabled: %v", err)
	}
}
