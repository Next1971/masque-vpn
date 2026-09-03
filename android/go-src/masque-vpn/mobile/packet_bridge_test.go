package mobile

import (
	"os"
	"testing"
)

func TestWritePacketReachesDeviceRead(t *testing.T) {
	tun := &Tunnel{bridge: newBridgeTUN(1400)}
	want := []byte{0x45, 0x00, 0x00, 0x14}
	if err := tun.WritePacket(want); err != nil {
		t.Fatal(err)
	}
	bufs := [][]byte{make([]byte, 16+64)}
	sizes := make([]int, 1)
	n, err := tun.bridge.Read(bufs, sizes, 16)
	if err != nil || n != 1 || sizes[0] != len(want) {
		t.Fatalf("read: n=%d size=%d err=%v", n, sizes[0], err)
	}
	got := bufs[0][16 : 16+sizes[0]]
	if string(got) != string(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDeviceWriteReachesReadPacket(t *testing.T) {
	tun := &Tunnel{bridge: newBridgeTUN(1400)}
	out := append(make([]byte, 16), 0x60, 0x00)
	if _, err := tun.bridge.Write([][]byte{out}, 16); err != nil {
		t.Fatal(err)
	}
	pkt := tun.ReadPacket()
	if string(pkt) != string([]byte{0x60, 0x00}) {
		t.Fatalf("ReadPacket: %v", pkt)
	}
}

func TestBridgeCloseUnblocksRead(t *testing.T) {
	br := newBridgeTUN(1400)
	if err := br.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := br.Read([][]byte{make([]byte, 64)}, make([]int, 1), 16)
	if err != os.ErrClosed {
		t.Fatalf("closed read: %v", err)
	}
	tun := &Tunnel{bridge: br}
	if tun.ReadPacket() != nil {
		t.Fatal("expected nil after close")
	}
}
