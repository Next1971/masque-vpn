package vpssetup

import (
	"fmt"
	"strconv"
	"strings"
)

// ExistingInstall is a MASQUE layout already on the VPS (do not reinstall).
type ExistingInstall struct {
	Present    bool
	Active     bool
	HasCA      bool
	ServerName string
	UDPPort    int
	Note       string
}

func (e ExistingInstall) Summary() string {
	if !e.Present {
		return "No existing MASQUE install"
	}
	host := e.ServerName
	if host == "" {
		host = "?"
	}
	port := "?"
	if e.UDPPort > 0 {
		port = strconv.Itoa(e.UDPPort)
	}
	state := "installed"
	if e.Active {
		state = "running"
	}
	ca := "CA ready"
	if !e.HasCA {
		ca = "CA key missing — cannot issue"
	}
	return fmt.Sprintf("MASQUE already %s — will not reinstall (%s UDP %s, %s)", state, host, port, ca)
}

// ParseServerTOML extracts server_name and UDP port from config.server.toml.
func ParseServerTOML(text string) (name string, port int) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch key {
		case "server_name":
			name = val
		case "bind":
			if i := strings.LastIndex(val, ":"); i >= 0 {
				if p, err := strconv.Atoi(val[i+1:]); err == nil {
					port = p
				}
			}
		}
	}
	return name, port
}

func parseMasqueProbe(block string) ExistingInstall {
	flags, toml, _ := strings.Cut(block, "---TOML---")
	toml, _, _ = strings.Cut(toml, "---ENDTOML---")
	var e ExistingInstall
	e.Active = strings.Contains(flags, "ACTIVE")
	e.HasCA = strings.Contains(flags, "HASCA")
	hasConfig := strings.Contains(flags, "HASCONFIG")
	hasBin := strings.Contains(flags, "HASBIN")
	hasSvc := strings.Contains(flags, "HASSVC")
	e.Present = hasConfig || e.Active || (hasBin && hasSvc)
	e.ServerName, e.UDPPort = ParseServerTOML(toml)
	if e.Present {
		e.Note = e.Summary()
	}
	return e
}
