//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"masque-client/internal/ipc"
)

func runServiceCLI(status, connect, disconnect bool, importPath string) error {
	if importPath != "" {
		b, err := os.ReadFile(importPath)
		if err != nil {
			return err
		}
		text := string(b)
		ca, cert, key := "", "", ""
		if !strings.Contains(text, "-----BEGIN") {
			dir := filepath.Dir(importPath)
			ca = readOptional(filepath.Join(dir, "certs", "ca.crt"))
			if ca == "" {
				ca = readOptional(filepath.Join(dir, "ca.crt"))
			}
			cert = readOptional(filepath.Join(dir, "certs", "client.crt"))
			if cert == "" {
				cert = readOptional(filepath.Join(dir, "client.crt"))
			}
			key = readOptional(filepath.Join(dir, "certs", "client.key"))
			if key == "" {
				key = readOptional(filepath.Join(dir, "client.key"))
			}
		}
		resp, err := ipc.RoundTrip(ipc.Request{
			Cmd:      ipc.CmdImport,
			Text:     text,
			Filename: filepath.Base(importPath),
			CA:       ca,
			Cert:     cert,
			Key:      key,
		})
		if err != nil {
			return err
		}
		fmt.Printf("imported: configured=%v\n", resp.Configured)
	}
	if connect {
		if _, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdConnect}); err != nil {
			return err
		}
	}
	if disconnect {
		if _, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdDisconnect}); err != nil {
			return err
		}
	}
	if status || (!connect && !disconnect && importPath == "") {
		resp, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdStatus})
		if err != nil {
			return err
		}
		fmt.Printf("state=%s configured=%v autoconnect=%v ip=%s\n%s\n",
			resp.State, resp.Configured, resp.Autoconnect, resp.AssignedIP, resp.Detail)
	}
	return nil
}

func readOptional(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
