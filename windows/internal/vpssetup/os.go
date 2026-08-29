package vpssetup

import (
	"bufio"
	"fmt"
	"strings"
)

// Distro is a parsed /etc/os-release plus uname -m.
type Distro struct {
	ID         string
	VersionID  string
	PrettyName string
	Arch       string // uname -m: x86_64, aarch64, ...
}

// ParseOSRelease parses the key=value format of /etc/os-release.
func ParseOSRelease(text string) Distro {
	d := Distro{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			d.ID = strings.ToLower(v)
		case "VERSION_ID":
			d.VersionID = v
		case "PRETTY_NAME":
			d.PrettyName = v
		}
	}
	return d
}

// GoArch maps uname -m to GOARCH used in vpn-server-linux-* filenames.
func GoArch(unameM string) (string, error) {
	switch strings.TrimSpace(unameM) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported CPU architecture %q (need x86_64 or aarch64)", unameM)
	}
}

// Supported reports whether this distro is one we install on (apt + systemd, tested).
func Supported(d Distro) error {
	if d.PrettyName == "" {
		d.PrettyName = d.ID + " " + d.VersionID
	}
	switch d.ID {
	case "ubuntu":
		switch d.VersionID {
		case "22.04", "24.04":
			return nil
		default:
			return fmt.Errorf("Ubuntu %s is not supported (need 22.04 or 24.04 LTS)", d.VersionID)
		}
	case "debian":
		if d.VersionID == "12" {
			return nil
		}
		return fmt.Errorf("Debian %s is not supported (need Debian 12)", d.VersionID)
	default:
		return fmt.Errorf("OS %q is not supported (need Ubuntu 22.04/24.04 or Debian 12)", d.PrettyName)
	}
}
