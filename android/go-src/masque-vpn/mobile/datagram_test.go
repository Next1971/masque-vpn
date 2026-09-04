package mobile

import (
	"os"
	"testing"
	"time"
)

type captureWriter struct {
	ch chan []byte
}

func (w *captureWriter) WriteDatagram(p []byte) error {
	cp := append([]byte(nil), p...)
	w.ch <- cp
	return nil
}

func TestDatagramPipeWriteToCopiesAndSends(t *testing.T) {
	w := &captureWriter{ch: make(chan []byte, 1)}
	pipe, err := NewDatagramPipe(w, "203.0.113.10", 2053)
	if err != nil {
		t.Fatal(err)
	}
	n, err := pipe.conn.WriteTo([]byte("quic"), nil)
	if err != nil || n != 4 {
		t.Fatalf("WriteTo n=%d err=%v", n, err)
	}
	got := <-w.ch
	if string(got) != "quic" {
		t.Fatalf("got %q", got)
	}
}

func TestDatagramPipeDeliverReadFrom(t *testing.T) {
	w := &captureWriter{ch: make(chan []byte, 1)}
	pipe, err := NewDatagramPipe(w, "203.0.113.10", 2053)
	if err != nil {
		t.Fatal(err)
	}
	pipe.Deliver([]byte("hello"))
	buf := make([]byte, 16)
	n, addr, err := pipe.conn.ReadFrom(buf)
	if err != nil || n != 5 || string(buf[:n]) != "hello" {
		t.Fatalf("ReadFrom n=%d err=%v buf=%q", n, err, buf[:n])
	}
	if addr.String() != "203.0.113.10:2053" {
		t.Fatalf("addr %s", addr)
	}
}

func TestDatagramPipeCloseUnblocksRead(t *testing.T) {
	w := &captureWriter{ch: make(chan []byte, 1)}
	pipe, err := NewDatagramPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := pipe.conn.ReadFrom(make([]byte, 8))
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := pipe.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected closed error")
		}
	case <-time.After(time.Second):
		t.Fatal("ReadFrom still blocked")
	}
}

func TestDatagramPipeReadDeadline(t *testing.T) {
	w := &captureWriter{ch: make(chan []byte, 1)}
	pipe, err := NewDatagramPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipe.conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, _, err = pipe.conn.ReadFrom(make([]byte, 8))
	if err != os.ErrDeadlineExceeded {
		t.Fatalf("deadline: %v", err)
	}
}

func TestNewDatagramPipeNilWriter(t *testing.T) {
	if _, err := NewDatagramPipe(nil, "203.0.113.10", 443); err == nil {
		t.Fatal("expected error")
	}
}

func TestDialWithPipeNilDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	_, err := DialWithPipe(&Config{Server: "203.0.113.10:2053", ServerName: "x"}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
