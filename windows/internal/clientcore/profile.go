// Package clientcore is the shared MASQUE client core for all platforms
// (Linux/Windows/Android). The core does NOT create TUN itself and does NOT
// touch routing — these platform-specific details are injected from outside
// by thin wrappers:
//   - Linux:   cmd/vpn-client (CreateTUN by name + ip route)
//   - Windows: wrapper around wintun + netsh (next stage)
//   - Android: TUN fd from VpnService + CreateTUNFromFile (next stage)
//
// This way, the same connection/forwarding/closing code is reused
// across all platforms — this is the "shared core" described in PROJECT.md.
package clientcore

import (
	"fmt"
	"net/netip"

	"github.com/BurntSushi/toml"
)

// Profile is the client server profile. The same set of parameters
// is used for both Android and Windows (PROJECT.md requirement). It is read
// from a TOML file, which is edited on the device through the UI.
//
// Secrets (the client's private key) are stored in the profile as a FILE PATH,
// not inline — so the profile can be displayed/logged without leaking secrets.
// (Later, the Android/Windows UI may store the key in secure storage.)
type Profile struct {
	// [server]
	Server     string `toml:"server"`      // host:port of the MASQUE proxy (UDP), e.g. "YOUR_SERVER_HOST:4433"
	ServerName string `toml:"server_name"` // TLS SNI / URI-template host, e.g. "YOUR_SERVER_HOST"

	// [tls] — mTLS material (paths to PEM files, NOT inline secrets)
	CA       string `toml:"ca"`       // CA for server certificate verification
	Cert     string `toml:"cert"`     // client certificate (mTLS)
	Key      string `toml:"key"`      // client private key (mTLS)
	Insecure bool   `toml:"insecure"` // if true, skip server certificate verification (INSECURE; troubleshooting only)

	// [tun]
	TUNName string   `toml:"tun_name"` // interface name (Linux/Windows), e.g. "masque0"
	MTU     int      `toml:"mtu"`      // tunnel MTU, e.g. 1400
	DNS     []string `toml:"dns"`      // DNS servers for the tunnel (full-route), default ["1.1.1.1"]
}

// tomlProfile is an intermediate struct for TOML sections.
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

// LoadProfile reads and validates the client's TOML profile.
// Unknown keys are treated as an error (protection against profile typos).
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
		// Allow hostname:port — ParseAddrPort requires an IP, so
		// strict validation is deferred to the Dial stage (ResolveUDPAddr).
		if !hasPort(p.Server) {
			return fmt.Errorf("profile: [server].server %q must be host:port", p.Server)
		}
	}
	if p.ServerName == "" {
		return fmt.Errorf("profile: [server].server_name is required (TLS SNI)")
	}
	if p.MTU == 0 {
		p.MTU = 1400 // reasonable default for QUIC/MASQUE
	}
	if p.MTU < 576 || p.MTU > 9000 {
		return fmt.Errorf("profile: [tun].mtu %d out of range (576..9000)", p.MTU)
	}
	if p.TUNName == "" {
		p.TUNName = "masque0"
	}
	if len(p.DNS) == 0 {
		p.DNS = []string{"1.1.1.1"} // reasonable default for the tunnel
	}
	// DNS address validation.
	for _, d := range p.DNS {
		if _, err := netip.ParseAddr(d); err != nil {
			return fmt.Errorf("profile: [tun].dns %q is not a valid IP: %w", d, err)
		}
	}
	return nil
}

// hasPort performs a rough check for a trailing ":port" in the string.
func hasPort(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i < len(s)-1
		}
	}
	return false
}
