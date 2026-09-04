package clientcore

import (
	"net"
	"testing"
)

func TestBindUDPEmptyName(t *testing.T) {
	if err := bindUDPToInterface(nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestBindUDPUnknownInterfaceDoesNotPanic(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("bind panicked: %v", r)
		}
	}()
	_ = bindUDPToInterface(conn, "masque-no-such-if")
}
