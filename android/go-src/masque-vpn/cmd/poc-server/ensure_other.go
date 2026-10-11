//go:build !linux

package main

import (
	"log"
	"os"
	"strings"
)

func ensureHTTP2Connect() {
	if strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		return
	}
	log.Printf("TCP CONNECT-IP needs GODEBUG=http2xconnect=1")
}
