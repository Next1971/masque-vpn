package mobile

import "testing"

func TestServerIPv4Literal(t *testing.T) {
	tun := &Tunnel{prof: profileFromConfig(&Config{
		Server:     "203.0.113.10:443",
		ServerName: "example.test",
		MTU:        1400,
	})}
	if got := tun.ServerIPv4(); got != "203.0.113.10" {
		t.Fatalf("ServerIPv4=%q", got)
	}
}
