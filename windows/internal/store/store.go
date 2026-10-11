package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"masque-client/internal/clientcore"
)

type Settings struct {
	Autoconnect bool   `json:"autoconnect"`
	KillSwitch  bool   `json:"kill_switch"`
	Transport   string `json:"transport,omitempty"`
	MTU         int    `json:"mtu,omitempty"`
}

func (s Settings) EffectiveTransport() string {
	return clientcore.TransportMode(s.Transport)
}

func (s Settings) EffectiveMTU() int {
	if s.MTU == 0 {
		return clientcore.TunnelMTU
	}
	return clientcore.ClampMTU(s.MTU)
}

func Dir() string {
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "MASQUE")
}

func profilePath() string  { return filepath.Join(Dir(), "profile.toml") }
func settingsPath() string { return filepath.Join(Dir(), "settings.json") }
func trafficPath() string  { return filepath.Join(Dir(), "traffic.json") }
func certsDir() string     { return filepath.Join(Dir(), "certs") }

func Configured() bool {
	_, err := os.Stat(profilePath())
	return err == nil
}

func Load() (*clientcore.Profile, error) {
	return clientcore.LoadProfile(profilePath())
}

func LoadSettings() Settings {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{}
	}
	var s Settings
	if json.Unmarshal(b, &s) != nil {
		return Settings{}
	}
	return s
}

func SaveSettings(s Settings) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath(), b, 0600)
}

func SetKillSwitch(v bool) error {
	s := LoadSettings()
	s.KillSwitch = v
	return SaveSettings(s)
}

func SetAutoconnect(v bool) error {
	s := LoadSettings()
	s.Autoconnect = v
	return SaveSettings(s)
}

func SetTransport(v string) error {
	s := LoadSettings()
	mode := clientcore.TransportMode(v)
	if mode == "auto" {
		s.Transport = ""
	} else {
		s.Transport = mode
	}
	return SaveSettings(s)
}

func SetMTU(v int) error {
	s := LoadSettings()
	s.MTU = clientcore.ClampMTU(v)
	return SaveSettings(s)
}

var trafficMu sync.Mutex

type trafficFile struct {
	DayKey     string `json:"day"`
	MonthKey   string `json:"month"`
	DayBytes   int64  `json:"day_bytes"`
	MonthBytes int64  `json:"month_bytes"`
	Applied    int64  `json:"applied"`
}

// ObserveTraffic folds the pump total into the local day and month counters.
// total is monotonic for the current process and may drop to zero after a restart.
func ObserveTraffic(total int64) (dayBytes, monthBytes int64) {
	trafficMu.Lock()
	defer trafficMu.Unlock()
	prev := loadTraffic()
	next := clientcore.NoteTraffic(clientcore.TrafficAccount{
		DayKey: prev.DayKey, MonthKey: prev.MonthKey,
		DayBytes: prev.DayBytes, MonthBytes: prev.MonthBytes, Applied: prev.Applied,
	}, total, time.Now())
	updated := trafficFile{
		DayKey: next.DayKey, MonthKey: next.MonthKey,
		DayBytes: next.DayBytes, MonthBytes: next.MonthBytes, Applied: next.Applied,
	}
	if updated != prev {
		_ = saveTraffic(updated)
	}
	return next.DayBytes, next.MonthBytes
}

func loadTraffic() trafficFile {
	b, err := os.ReadFile(trafficPath())
	if err != nil {
		return trafficFile{}
	}
	var f trafficFile
	if json.Unmarshal(b, &f) != nil {
		return trafficFile{}
	}
	return f
}

func saveTraffic(f trafficFile) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(trafficPath(), b, 0600)
}

// Import writes a profile into ProgramData. text is the file contents;
// extra PEM blobs fill path-based toml that does not inline certificates.
func Import(text, filename, extraCA, extraCert, extraKey string) error {
	p, ca, cert, key, err := parseBundle(text, extraCA, extraCert, extraKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(certsDir(), 0700); err != nil {
		return err
	}
	caPath := filepath.Join(certsDir(), "ca.crt")
	certPath := filepath.Join(certsDir(), "client.crt")
	keyPath := filepath.Join(certsDir(), "client.key")
	if err := os.WriteFile(caPath, []byte(ensureNL(ca)), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(certPath, []byte(ensureNL(cert)), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, []byte(ensureNL(key)), 0600); err != nil {
		return err
	}
	if p.TUNName == "" {
		p.TUNName = "masque0"
	}
	// The profile file is not the MTU setting. Connect reads settings.json.
	p.MTU = clientcore.TunnelMTU
	if len(p.DNS) == 0 {
		p.DNS = []string{"1.1.1.1"}
	}
	p.CA = caPath
	p.Cert = certPath
	p.Key = keyPath
	if err := p.Validate(); err != nil {
		return err
	}
	return writeProfileTOML(profilePath(), p)
}

func writeProfileTOML(path string, p *clientcore.Profile) error {
	dns := make([]string, len(p.DNS))
	for i, d := range p.DNS {
		dns[i] = fmt.Sprintf("%q", d)
	}
	alt := ""
	if p.AltPort > 0 {
		alt = fmt.Sprintf("alt_port = %d\n", p.AltPort)
	}
	body := fmt.Sprintf(`[server]
server = %q
server_name = %q
%s
[tls]
ca = %q
cert = %q
key = %q
insecure = %t

[tun]
tun_name = %q
mtu = %d
dns = [%s]
`, p.Server, p.ServerName, alt, p.CA, p.Cert, p.Key, p.Insecure, p.TUNName, p.MTU, strings.Join(dns, ", "))
	return os.WriteFile(path, []byte(body), 0600)
}

func parseBundle(text, extraCA, extraCert, extraKey string) (*clientcore.Profile, string, string, string, error) {
	p := &clientcore.Profile{}

	if strings.Contains(text, "[server]") || strings.Contains(text, "[tls]") {
		var tp struct {
			Server struct {
				Server     string `toml:"server"`
				ServerName string `toml:"server_name"`
				AltPort    int    `toml:"alt_port"`
			} `toml:"server"`
			TLS struct {
				CA       string `toml:"ca"`
				Cert     string `toml:"cert"`
				Key      string `toml:"key"`
				Insecure bool   `toml:"insecure"`
			} `toml:"tls"`
			TUN struct {
				Name string   `toml:"tun_name"`
				MTU  int      `toml:"mtu"`
				DNS  []string `toml:"dns"`
			} `toml:"tun"`
		}
		if err := toml.Unmarshal([]byte(text), &tp); err == nil {
			p.Server = tp.Server.Server
			p.ServerName = tp.Server.ServerName
			p.AltPort = tp.Server.AltPort
			p.Insecure = tp.TLS.Insecure
			p.TUNName = tp.TUN.Name
			p.MTU = tp.TUN.MTU
			p.DNS = tp.TUN.DNS
			ca := pickPEM(tp.TLS.CA, extraCA)
			cert := pickPEM(tp.TLS.Cert, extraCert)
			key := pickPEM(tp.TLS.Key, extraKey)
			if p.Server == "" || p.ServerName == "" {
				return nil, "", "", "", fmt.Errorf("profile missing [server] fields")
			}
			if !looksPEM(ca) || !looksPEM(cert) || !looksPEM(key) {
				return nil, "", "", "", fmt.Errorf("profile certificates must be inline PEM or provided alongside the toml")
			}
			return p, ca, cert, key, nil
		}
	}

	server := firstString(text, "address", "server")
	name := firstString(text, "name", "server_name")
	dns := firstString(text, "dns")
	ca := pickPEM(triple(text, "ca"), extraCA)
	cert := pickPEM(triple(text, "cert"), extraCert)
	key := pickPEM(triple(text, "key"), extraKey)
	if server == "" {
		return nil, "", "", "", fmt.Errorf("profile missing server address")
	}
	if name == "" {
		if i := strings.LastIndex(server, ":"); i > 0 {
			name = server[:i]
		} else {
			name = server
		}
	}
	p.Server = server
	p.ServerName = name
	p.AltPort = firstInt(text, "alt_port")
	if dns != "" {
		p.DNS = []string{dns}
	}
	if !looksPEM(ca) || !looksPEM(cert) || !looksPEM(key) {
		return nil, "", "", "", fmt.Errorf("profile missing tls.ca / tls.cert / tls.key PEM")
	}
	return p, ca, cert, key, nil
}

func pickPEM(primary, extra string) string {
	primary = strings.TrimSpace(primary)
	if looksPEM(primary) {
		return primary
	}
	return strings.TrimSpace(extra)
}

func looksPEM(s string) bool {
	return strings.Contains(s, "-----BEGIN")
}

func ensureNL(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func firstInt(text string, keys ...string) int {
	for _, key := range keys {
		if s := firstString(text, key); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				return n
			}
		}
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*(\d+)`)
		if m := re.FindStringSubmatch(text); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n
			}
		}
	}
	return 0
}

func firstString(text string, keys ...string) string {
	for _, key := range keys {
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
		if m := re.FindStringSubmatch(text); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func triple(text, key string) string {
	re := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(key) + `\s*=\s*"{3}(.*?)"{3}`)
	if m := re.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
