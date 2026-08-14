// config.go reads server configuration from TOML (E1).
// Flags remain overrides/fallbacks: if -config is not set, flags are used.
package main

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Config is the server config.toml structure.
type Config struct {
	Server  ServerSection  `toml:"server"`
	TLS     TLSSection     `toml:"tls"`
	TUN     TUNSection     `toml:"tun"`
	Network NetworkSection `toml:"network"`
}

type ServerSection struct {
	Bind       string `toml:"bind"`
	ServerName string `toml:"server_name"`
}

type TLSSection struct {
	Cert     string `toml:"cert"`
	Key      string `toml:"key"`
	ClientCA string `toml:"client_ca"` // empty = no mTLS
}

type TUNSection struct {
	Name string `toml:"name"`
	MTU  int    `toml:"mtu"`
}

type NetworkSection struct {
	TunAddr  string `toml:"tun_addr"`  // server tunnel address, e.g. 10.8.0.1/24
	PoolCIDR string `toml:"pool_cidr"` // client assignment pool, e.g. 10.8.0.0/24
	Route    string `toml:"route"`     // route advertised to the client, e.g. 0.0.0.0/0
}

// LoadConfig reads and validates config.toml.
func LoadConfig(path string) (*Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return nil, fmt.Errorf("unknown keys in config: %v", undec)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Server.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	if c.Server.ServerName == "" {
		return fmt.Errorf("server.server_name is required")
	}
	if c.TLS.Cert == "" || c.TLS.Key == "" {
		return fmt.Errorf("tls.cert and tls.key are required")
	}
	if c.TUN.Name == "" {
		return fmt.Errorf("tun.name is required")
	}
	if c.TUN.MTU <= 0 {
		c.TUN.MTU = 1400
	}
	if c.Network.TunAddr == "" {
		return fmt.Errorf("network.tun_addr is required")
	}
	if c.Network.PoolCIDR == "" {
		return fmt.Errorf("network.pool_cidr is required")
	}
	if c.Network.Route == "" {
		c.Network.Route = "0.0.0.0/0"
	}
	return nil
}
