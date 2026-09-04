// Package clientcore is the shared MASQUE client core for all platforms
// (Linux/Windows/Android). The core does NOT create TUN itself or modify routes;
// these platform details are injected externally by thin wrappers:
//   - Linux:   cmd/vpn-client (CreateTUN by name + ip route)
//   - Windows: wintun + netsh wrapper (next stage)
//   - Android: TUN fd from VpnService + CreateTUNFromFile (next stage)
//
// This reuses the same connection/forwarding/closure code
// across all platforms—the “single core” described in PROJECT.md.
package clientcore

import (
	"fmt"
	"net/netip"

	"github.com/BurntSushi/toml"
)

// Profile is the client server profile. The same parameter set is used
// for Android and Windows (a PROJECT.md requirement). It is read from a TOML file
// that is edited through the device UI.
//
// Secrets (the client private key) are stored in the profile as a file PATH,
// not inline, so the profile can be displayed/logged without leakage.
// (The Android/Windows UI can later store the key in secure storage.)
type Profile struct {
	// [server]
	Server     string `toml:"server"`      // MASQUE proxy host:port (UDP), e.g. "YOUR_SERVER_HOST:4433"
	ServerName string `toml:"server_name"` // TLS SNI / URI-template host, e.g. "YOUR_SERVER_HOST"

	// [tls] — mTLS material (paths to PEM files, NOT inline secrets)
	CA       string `toml:"ca"`       // CA for server certificate verification
	Cert     string `toml:"cert"`     // client certificate (mTLS)
	Key      string `toml:"key"`      // client private key (mTLS)
	Insecure bool   `toml:"insecure"` // if true, skip server certificate verification (INSECURE; troubleshooting only)

	// [tun]
	TUNName string   `toml:"tun_name"` // interface name (Linux/Windows), e.g. "masque0"
	MTU     int      `toml:"mtu"`      // tunnel MTU, e.g. 1400
	DNS     []string `toml:"dns"`      // tunnel DNS servers (full-route), default ["1.1.1.1"]

	// BindInterface, if set, binds the QUIC UDP socket to that network
	// interface (iOS Packet Tunnel: Wi‑Fi/LTE so handshake is not swallowed by utun).
	BindInterface string `toml:"-"`
}

// tomlProfile is an intermediate structure for TOML sections.
type tomlProfile struct {
	Server struct {
		Server     string `toml:"server"`
		ServerName string `toml:"server_name"`
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

// LoadProfile reads and validates the client TOML profile.
// Unknown keys are errors (protection against profile typos).
func LoadProfile(path string) (*Profile, error) {
	var tp tomlProfile
	md, err := toml.DecodeFile(path, &tp)
	if err != nil {
		return nil, fmt.Errorf("decode profile %q: %w", path, err)
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return nil, fmt.Errorf("profile %q has unknown keys: %v", path, undec)
	}

	p := &Profile{
		Server:     tp.Server.Server,
		ServerName: tp.Server.ServerName,
		CA:         tp.TLS.CA,
		Cert:       tp.TLS.Cert,
		Key:        tp.TLS.Key,
		Insecure:   tp.TLS.Insecure,
		TUNName:    tp.TUN.Name,
		MTU:        tp.TUN.MTU,
		DNS:        tp.TUN.DNS,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Validate checks required profile fields.
func (p *Profile) Validate() error {
	if p.Server == "" {
		return fmt.Errorf("profile: [server].server is required (host:port)")
	}
	if _, err := netip.ParseAddrPort(p.Server); err != nil {
		// hostname:port is accepted—ParseAddrPort requires an IP, so
		// strict validation is deferred to Dial (ResolveUDPAddr).
		if !hasPort(p.Server) {
			return fmt.Errorf("profile: [server].server %q must be host:port", p.Server)
		}
	}
	if p.ServerName == "" {
		return fmt.Errorf("profile: [server].server_name is required (TLS SNI)")
	}
	if p.MTU == 0 {
		p.MTU = 1400 // sensible default for QUIC/MASQUE
	}
	if p.MTU < 576 || p.MTU > 9000 {
		return fmt.Errorf("profile: [tun].mtu %d out of range (576..9000)", p.MTU)
	}
	if p.TUNName == "" {
		p.TUNName = "masque0"
	}
	if len(p.DNS) == 0 {
		p.DNS = []string{"1.1.1.1"} // sensible default for the tunnel
	}
	// Validate DNS addresses.
	for _, d := range p.DNS {
		if _, err := netip.ParseAddr(d); err != nil {
			return fmt.Errorf("profile: [tun].dns %q is not a valid IP: %w", d, err)
		}
	}
	return nil
}

// hasPort roughly checks for a trailing ":port".
func hasPort(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i < len(s)-1
		}
	}
	return false
}
