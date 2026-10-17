package vpssetup

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/install.sh
var installScript []byte

//go:embed assets/gen-config.sh
var genConfigScript []byte

const (
	remoteBin       = "/tmp/masque-vpn-server-bin"
	remoteGenConfig = "/tmp/masque-gen-config.sh"
	remoteInstall   = "/tmp/masque-install.sh"
)

// Logf is a progress logger for the GUI.
type Logf func(format string, args ...interface{})

// Preflight is the result of OS / port / ufw checks before install.
type Preflight struct {
	Distro       Distro
	GoArch       string
	Pretty       string
	HasUFW       bool
	Listening    map[int]struct{}
	ListeningTCP map[int]struct{}
	Recommended  []int
	TakenSuggest []int
	TakenTCP     []int
	Existing     ExistingInstall
}

// CheckOSAndMachine refuses unsupported distros, non-systemd, missing TUN, or odd CPU.
func CheckOSAndMachine(c *Client, log Logf) (Preflight, error) {
	var z Preflight
	out, err := c.Run(30*time.Second, `set -e
if [ "$(id -u)" -ne 0 ]; then echo 'ERR notroot: log in as root'; exit 1; fi
if [ ! -d /run/systemd/system ]; then echo 'ERR nosystemd'; exit 1; fi
if [ ! -e /dev/net/tun ]; then echo 'ERR notun'; exit 1; fi
command -v apt-get >/dev/null || { echo 'ERR noapt'; exit 1; }
echo '---OS---'
cat /etc/os-release
echo '---ARCH---'
uname -m
echo '---UFW---'
if command -v ufw >/dev/null 2>&1; then echo HASUFW; else echo NOUFW; fi
echo '---SS---'
ss -H -uln 2>/dev/null || true
echo '---SSTCP---'
ss -H -tln 2>/dev/null || true
echo '---MASQUE---'
if [ -f /opt/masque/config.server.toml ]; then echo HASCONFIG; else echo NOCONFIG; fi
if [ -f /opt/masque/vpn-server ] || [ -x /opt/masque/bin/poc-server ]; then echo HASBIN; else echo NOBIN; fi
if [ -f /etc/systemd/system/masque.service ] || [ -f /lib/systemd/system/masque.service ]; then echo HASSVC; else echo NOSVC; fi
if systemctl is-active --quiet masque 2>/dev/null; then echo ACTIVE; else echo INACTIVE; fi
if [ ! -f /opt/masque/ca/ca.key ]; then
  for src in /opt/masque/generated/ca /opt/masque/out2/ca /opt/masque/out/ca; do
    if [ -f "$src/ca.key" ] && [ -f "$src/ca.crt" ]; then
      install -d -m 0755 /opt/masque/ca
      install -m 0644 "$src/ca.crt" /opt/masque/ca/ca.crt
      install -m 0600 "$src/ca.key" /opt/masque/ca/ca.key
      echo ADOPTED_CA
      break
    fi
  done
fi
if [ -f /opt/masque/ca/ca.key ]; then echo HASCA; else echo NOCA; fi
if [ -f /opt/masque/state/alt_port ]; then echo "ALTPORT $(tr -d '[:space:]' < /opt/masque/state/alt_port)"; else echo NOALT; fi
echo '---TOML---'
cat /opt/masque/config.server.toml 2>/dev/null || true
echo '---ENDTOML---'
`, nil)
	if err != nil {
		return z, fmt.Errorf("preflight: %w", err)
	}
	osPart, rest, _ := strings.Cut(out, "---ARCH---")
	osPart = strings.TrimPrefix(strings.TrimSpace(osPart), "---OS---")
	archPart, rest, _ := strings.Cut(strings.TrimSpace(rest), "---UFW---")
	ufwPart, rest, _ := strings.Cut(strings.TrimSpace(rest), "---SS---")
	ssPart, rest, _ := strings.Cut(rest, "---SSTCP---")
	tcpPart, masquePart, _ := strings.Cut(rest, "---MASQUE---")

	d := ParseOSRelease(osPart)
	d.Arch = strings.TrimSpace(archPart)
	if err := Supported(d); err != nil {
		return z, err
	}
	goarch, err := GoArch(d.Arch)
	if err != nil {
		return z, err
	}
	hasUFW := strings.Contains(ufwPart, "HASUFW")
	listening := ParseListeningUDPPorts(ssPart)
	listeningTCP := ParseListeningUDPPorts(tcpPart)
	rec := RecommendFree(listening, listeningTCP)
	var taken, takenTCP []int
	for _, p := range CandidatePorts {
		if _, ok := listening[p]; ok {
			taken = append(taken, p)
		}
		if _, ok := listeningTCP[p]; ok {
			takenTCP = append(takenTCP, p)
		}
	}
	z = Preflight{
		Distro:       d,
		GoArch:       goarch,
		Pretty:       d.PrettyName,
		HasUFW:       hasUFW,
		Listening:    listening,
		ListeningTCP: listeningTCP,
		Recommended:  rec,
		TakenSuggest: taken,
		TakenTCP:     takenTCP,
		Existing:     parseMasqueProbe(masquePart),
	}
	if log != nil {
		log("OS: %s (%s)", d.PrettyName, goarch)
		if hasUFW {
			log("ufw: present (will allow UDP and TCP)")
		} else {
			log("ufw: not installed (will skip firewall rules)")
		}
		log("Candidates free on both UDP and TCP: %s", formatPortList(rec))
		if len(taken) > 0 {
			log("UDP already listening: %s", formatPortList(taken))
		}
		if len(takenTCP) > 0 {
			log("TCP already listening: %s", formatPortList(takenTCP))
		}
		if z.Existing.Present {
			log("%s", z.Existing.Summary())
			if strings.Contains(masquePart, "ADOPTED_CA") {
				log("Copied CA into /opt/masque/ca from generated/out2/out (server cert not changed)")
			}
		}
	}
	return z, nil
}

// DefaultLinuxBinaryName is vpn-server-linux-amd64 or arm64.
func DefaultLinuxBinaryName(goarch string) string {
	return "vpn-server-linux-" + goarch
}

// FindLinuxBinary looks at userPath, then dirs (exe dir, cwd) for vpn-server-linux-$goarch.
func FindLinuxBinary(goarch, userPath string, searchDirs []string) (string, error) {
	if strings.TrimSpace(userPath) != "" {
		if _, err := os.Stat(userPath); err != nil {
			return "", fmt.Errorf("server binary: %w", err)
		}
		return userPath, nil
	}
	name := DefaultLinuxBinaryName(goarch)
	for _, dir := range searchDirs {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("place %s next to this app, or choose the file (build artifacts / GitHub release)", name)
}

// Install uploads the Linux server binary and runs the remote install script.
// altPort 0 installs a single listen port. A positive altPort is redirected
// to udpPort for UDP and TCP and stored for later profile issuance.
func Install(c *Client, publicHost string, udpPort, altPort int, linuxBin []byte, log Logf) error {
	if err := ValidateHost(publicHost); err != nil {
		return err
	}
	if err := ValidatePortPair(udpPort, altPort); err != nil {
		return err
	}
	if ex, err := detectExisting(c); err != nil {
		return err
	} else if ex.Present {
		return fmt.Errorf("%s", ex.Summary())
	}
	if len(linuxBin) < 1024 {
		return fmt.Errorf("server binary looks empty")
	}
	if log != nil {
		if altPort > 0 {
			log("Listen UDP/TCP %d, second port %d (redirect and alt_port)", udpPort, altPort)
		} else {
			log("Listen UDP/TCP %d, no second port", udpPort)
		}
		log("Uploading server binary (%d bytes)…", len(linuxBin))
	}
	if err := c.Upload(remoteBin, linuxBin, 0o755); err != nil {
		return err
	}
	if log != nil {
		log("Uploading gen-config and install scripts…")
	}
	if err := c.Upload(remoteGenConfig, genConfigScript, 0o755); err != nil {
		return err
	}
	if err := c.Upload(remoteInstall, installScript, 0o755); err != nil {
		return err
	}
	altEnv := ""
	if altPort > 0 {
		altEnv = fmt.Sprintf(" MASQUE_ALT_PORT=%d", altPort)
	}
	cmd := fmt.Sprintf("MASQUE_PORT=%d%s MASQUE_HOST=%s MASQUE_BIN=%s GEN_CONFIG=%s bash %s",
		udpPort,
		altEnv,
		shellSingleQuote(publicHost),
		shellSingleQuote(remoteBin),
		shellSingleQuote(remoteGenConfig),
		shellSingleQuote(remoteInstall),
	)
	if log != nil {
		log("Installing packages, certificates, systemd, ufw…")
	}
	out, err := c.Run(15*time.Minute, cmd, nil)
	if log != nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				log("%s", line)
			}
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func detectExisting(c *Client) (ExistingInstall, error) {
	out, err := c.Run(20*time.Second, `set +e
echo '---MASQUE---'
if [ -f /opt/masque/config.server.toml ]; then echo HASCONFIG; else echo NOCONFIG; fi
if [ -f /opt/masque/vpn-server ] || [ -x /opt/masque/bin/poc-server ]; then echo HASBIN; else echo NOBIN; fi
if [ -f /etc/systemd/system/masque.service ] || [ -f /lib/systemd/system/masque.service ]; then echo HASSVC; else echo NOSVC; fi
if systemctl is-active --quiet masque 2>/dev/null; then echo ACTIVE; else echo INACTIVE; fi
if [ -f /opt/masque/ca/ca.key ]; then echo HASCA; else echo NOCA; fi
if [ -f /opt/masque/state/alt_port ]; then echo "ALTPORT $(tr -d '[:space:]' < /opt/masque/state/alt_port)"; else echo NOALT; fi
echo '---TOML---'
cat /opt/masque/config.server.toml 2>/dev/null || true
echo '---ENDTOML---'
`, nil)
	if err != nil {
		return ExistingInstall{}, err
	}
	_, block, _ := strings.Cut(out, "---MASQUE---")
	return parseMasqueProbe(block), nil
}
