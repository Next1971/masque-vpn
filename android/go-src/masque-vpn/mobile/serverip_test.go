package mobile

import "testing"

func TestServerIPLiteralV6(t *testing.T) {
	tun := &Tunnel{prof: profileFromConfig(&Config{
		Server:     "[2001:db8::10]:443",
		ServerName: "example.test",
		MTU:        1400,
	})}
	if got := tun.ServerIP(); got != "2001:db8::10" {
		t.Fatalf("ServerIP=%q", got)
	}
}

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
