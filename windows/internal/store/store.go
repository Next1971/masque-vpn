package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// Stored profiles live one directory each under profiles\<id>\. The directory
// holds profile.toml plus the PEM files it points at, and profiles\active
// names the profile Connect uses. settings.json and traffic.json stay global
// per device; transport, MTU, and kill switch are not per-profile settings.
func profilesDir() string { return filepath.Join(Dir(), "profiles") }
func activePath() string  { return filepath.Join(profilesDir(), "active") }

// Info describes one stored profile for pickers and logs.
type Info struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Server string `json:"server"`
	Active bool   `json:"active"`
}

// Migrate moves a pre-2.2 single-profile install (profile.toml plus certs\
// beside it) into profiles\<id>\ and activates it. The legacy profile.toml is
// removed after a successful move, so a rerun finds nothing to migrate.
func Migrate() error {
	if _, err := os.Stat(profilePath()); err != nil {
		return nil
	}
	prof, err := clientcore.LoadProfile(profilePath())
	if err != nil {
		return fmt.Errorf("read legacy profile: %w", err)
	}
	caPEM, err := readPEMFile(prof.CA)
	if err != nil {
		return fmt.Errorf("migrate legacy profile: %w", err)
	}
	certPEM, err := readPEMFile(prof.Cert)
	if err != nil {
		return fmt.Errorf("migrate legacy profile: %w", err)
	}
	keyPEM, err := readPEMFile(prof.Key)
	if err != nil {
		return fmt.Errorf("migrate legacy profile: %w", err)
	}
	id := ProfileID(prof.ServerName, certPEM)
	dir := profileDir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "ca.crt"), caPEM); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "client.crt"), certPEM); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "client.key"), keyPEM); err != nil {
		return err
	}
	if prof.TUNName == "" {
		prof.TUNName = "masque0"
	}
	if len(prof.DNS) == 0 {
		prof.DNS = []string{"1.1.1.1"}
	}
	prof.CA = filepath.Join(dir, "ca.crt")
	prof.Cert = filepath.Join(dir, "client.crt")
	prof.Key = filepath.Join(dir, "client.key")
	if err := writeProfileTOML(filepath.Join(dir, "profile.toml"), prof); err != nil {
		return err
	}
	if err := setActive(id); err != nil {
		return err
	}
	if err := os.Remove(profilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.RemoveAll(filepath.Join(Dir(), "certs"))
	return nil
}

// ProfileID derives the storage id from the display name and the client
// certificate. The certificate hash keeps two profiles on the same server
// (two devices) apart, and makes re-importing the same bundle update that
// entry in place instead of creating a duplicate.
func ProfileID(name, certPEM string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(certPEM)))
	return slugify(name) + "-" + hex.EncodeToString(sum[:])[:8]
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		out = "profile"
	}
	return out
}

func profileDir(id string) string { return filepath.Join(profilesDir(), id) }

func readPEMFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writePEM(path, pem string) error {
	return os.WriteFile(path, []byte(ensureNL(pem)), 0600)
}

func setActive(id string) error {
	if err := os.MkdirAll(profilesDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(activePath(), []byte(id+"\n"), 0600)
}

// storedIDs lists profile directories that contain a profile.toml, sorted.
func storedIDs() []string {
	entries, err := os.ReadDir(profilesDir())
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(profilesDir(), e.Name(), "profile.toml")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

// ActiveID returns the id of the active profile. A stale pointer (deleted
// directory) falls back to the first stored profile so the client keeps a
// usable selection.
func ActiveID() (string, error) {
	ids := storedIDs()
	if len(ids) == 0 {
		return "", fmt.Errorf("no profiles stored")
	}
	id := ""
	if b, err := os.ReadFile(activePath()); err == nil {
		id = strings.TrimSpace(string(b))
	}
	for _, known := range ids {
		if known == id {
			return id, nil
		}
	}
	_ = setActive(ids[0])
	return ids[0], nil
}

// ActiveName returns the display name of the active profile, or "" when none.
func ActiveName() string {
	id, err := ActiveID()
	if err != nil {
		return ""
	}
	p, err := clientcore.LoadProfile(filepath.Join(profileDir(id), "profile.toml"))
	if err != nil {
		return ""
	}
	return p.ServerName
}

// List returns every stored profile. A profile whose profile.toml no longer
// parses still appears, marked "(invalid)", so it can be selected and deleted.
func List() ([]Info, error) {
	if err := Migrate(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(profilesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	active, _ := ActiveID()
	var out []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info := Info{ID: e.Name(), Active: e.Name() == active}
		if p, err := clientcore.LoadProfile(filepath.Join(profilesDir(), e.Name(), "profile.toml")); err == nil {
			info.Name = p.ServerName
			info.Server = p.Server
		} else {
			info.Name = "(invalid)"
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SetActive switches the active profile by id.
func SetActive(id string) error {
	if err := Migrate(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(profileDir(id), "profile.toml")); err != nil {
		return fmt.Errorf("no such profile")
	}
	return setActive(id)
}

// Delete removes one stored profile. Deleting the active profile reactivates
// the first remaining one, or clears the pointer when none are left.
func Delete(id string) error {
	if err := Migrate(); err != nil {
		return err
	}
	if _, err := os.Stat(profileDir(id)); err != nil {
		return fmt.Errorf("no such profile")
	}
	if err := os.RemoveAll(profileDir(id)); err != nil {
		return err
	}
	if active, err := ActiveID(); err == nil && active != id {
		return nil
	}
	ids := storedIDs()
	if len(ids) == 0 {
		_ = os.Remove(activePath())
		return nil
	}
	return setActive(ids[0])
}

func Configured() bool {
	if err := Migrate(); err != nil {
		return false
	}
	_, err := ActiveID()
	return err == nil
}

func Load() (*clientcore.Profile, error) {
	if err := Migrate(); err != nil {
		return nil, err
	}
	id, err := ActiveID()
	if err != nil {
		return nil, err
	}
	return clientcore.LoadProfile(filepath.Join(profileDir(id), "profile.toml"))
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

// Import parses a profile bundle and stores it under profiles\<id>\. An
// existing entry with the same server name and client certificate is updated
// in place; otherwise a new profile is added. The imported profile becomes
// the active one. text is the file contents; extra PEM blobs fill path-based
// toml that does not inline certificates.
func Import(text, filename, extraCA, extraCert, extraKey string) error {
	if err := Migrate(); err != nil {
		return err
	}
	p, ca, cert, key, err := parseBundle(text, extraCA, extraCert, extraKey)
	if err != nil {
		return err
	}
	id := ProfileID(p.ServerName, cert)
	dir := profileDir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "ca.crt"), ca); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "client.crt"), cert); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "client.key"), key); err != nil {
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
	p.CA = filepath.Join(dir, "ca.crt")
	p.Cert = filepath.Join(dir, "client.crt")
	p.Key = filepath.Join(dir, "client.key")
	if err := p.Validate(); err != nil {
		return err
	}
	if err := writeProfileTOML(filepath.Join(dir, "profile.toml"), p); err != nil {
		return err
	}
	return setActive(id)
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
	if p, ca, cert, key, ok := parseBundleTOML(text); ok {
		return p, pickPEM(ca, extraCA), pickPEM(cert, extraCert), pickPEM(key, extraKey), nil
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
	p := &clientcore.Profile{}
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

// parseBundleTOML reads a [server]/[tls]/[tun] profile. It accepts both the
// path-based toml ([server].server) and the self-contained gen-config.sh
// bundle ([server].address, dns as a bare string or a list). ok is false when
// the text does not decode into a complete profile, so the flat fallback in
// parseBundle can try plain key=value lines.
func parseBundleTOML(text string) (*clientcore.Profile, string, string, string, bool) {
	type serverSpec struct {
		Server     string `toml:"server"`
		Address    string `toml:"address"`
		ServerName string `toml:"server_name"`
		Name       string `toml:"name"`
		AltPort    int    `toml:"alt_port"`
	}
	type tlsSpec struct {
		CA       string `toml:"ca"`
		Cert     string `toml:"cert"`
		Key      string `toml:"key"`
		Insecure bool   `toml:"insecure"`
	}
	type tunStrings struct {
		Name string   `toml:"tun_name"`
		MTU  int      `toml:"mtu"`
		DNS  []string `toml:"dns"`
	}
	type tunScalar struct {
		Name string `toml:"tun_name"`
		MTU  int    `toml:"mtu"`
		DNS  string `toml:"dns"`
	}
	apply := func(spec serverSpec, tls tlsSpec, tunName string, tunMTU int, dns []string) (*clientcore.Profile, string, string, string, bool) {
		server := spec.Server
		if server == "" {
			server = spec.Address
		}
		name := spec.ServerName
		if name == "" {
			name = spec.Name
		}
		if server == "" || name == "" || !looksPEM(tls.CA) || !looksPEM(tls.Cert) || !looksPEM(tls.Key) {
			return nil, "", "", "", false
		}
		p := &clientcore.Profile{
			Server: server, ServerName: name, AltPort: spec.AltPort, Insecure: tls.Insecure,
			TUNName: tunName, MTU: tunMTU, DNS: dns,
		}
		return p, tls.CA, tls.Cert, tls.Key, true
	}

	var list struct {
		Server serverSpec `toml:"server"`
		TLS    tlsSpec    `toml:"tls"`
		TUN    tunStrings `toml:"tun"`
	}
	if err := toml.Unmarshal([]byte(text), &list); err == nil {
		if p, ca, cert, key, ok := apply(list.Server, list.TLS, list.TUN.Name, list.TUN.MTU, list.TUN.DNS); ok {
			return p, ca, cert, key, true
		}
	}
	var scalar struct {
		Server serverSpec `toml:"server"`
		TLS    tlsSpec    `toml:"tls"`
		TUN    tunScalar  `toml:"tun"`
	}
	if err := toml.Unmarshal([]byte(text), &scalar); err == nil {
		var dns []string
		if s := strings.TrimSpace(scalar.TUN.DNS); s != "" {
			dns = []string{s}
		}
		if p, ca, cert, key, ok := apply(scalar.Server, scalar.TLS, scalar.TUN.Name, scalar.TUN.MTU, dns); ok {
			return p, ca, cert, key, true
		}
	}
	return nil, "", "", "", false
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
