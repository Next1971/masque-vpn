package mobile

import (
	"os"
	"testing"
	"time"
)

type captureStream struct {
	ch chan []byte
}

func (w *captureStream) WriteStream(p []byte) error {
	cp := append([]byte(nil), p...)
	w.ch <- cp
	return nil
}

func TestStreamPipeWriteCopiesAndSends(t *testing.T) {
	w := &captureStream{ch: make(chan []byte, 1)}
	pipe, err := NewStreamPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	n, err := pipe.conn.Write([]byte("tls"))
	if err != nil || n != 3 {
		t.Fatalf("Write n=%d err=%v", n, err)
	}
	got := <-w.ch
	if string(got) != "tls" {
		t.Fatalf("got %q", got)
	}
}

func TestStreamPipeDeliverRead(t *testing.T) {
	w := &captureStream{ch: make(chan []byte, 1)}
	pipe, err := NewStreamPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	pipe.Deliver([]byte("hello-world"))
	buf := make([]byte, 5)
	n, err := pipe.conn.Read(buf)
	if err != nil || n != 5 || string(buf) != "hello" {
		t.Fatalf("Read n=%d err=%v buf=%q", n, err, buf)
	}
	n, err = pipe.conn.Read(buf)
	if err != nil || n != 5 || string(buf[:n]) != "-worl" {
		t.Fatalf("leftover n=%d err=%v buf=%q", n, err, buf[:n])
	}
}

func TestStreamPipeCloseUnblocksRead(t *testing.T) {
	w := &captureStream{ch: make(chan []byte, 1)}
	pipe, err := NewStreamPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := pipe.conn.Read(make([]byte, 8))
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
		t.Fatal("Read still blocked")
	}
}

func TestStreamPipeReadDeadline(t *testing.T) {
	w := &captureStream{ch: make(chan []byte, 1)}
	pipe, err := NewStreamPipe(w, "203.0.113.10", 443)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipe.conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, err = pipe.conn.Read(make([]byte, 8))
	if err != os.ErrDeadlineExceeded {
		t.Fatalf("deadline: %v", err)
	}
}

func TestNewStreamPipeNilWriter(t *testing.T) {
	if _, err := NewStreamPipe(nil, "203.0.113.10", 443); err == nil {
		t.Fatal("expected error")
	}
}

func TestDialWithStreamNilDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	_, err := DialWithStream(&Config{Server: "203.0.113.10:443", ServerName: "x", Transport: "tcp"}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
