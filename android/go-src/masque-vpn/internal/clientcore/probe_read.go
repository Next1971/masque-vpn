package clientcore

import (
	"fmt"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
)

func readPacketTimeout(conn *connectip.Conn, buf []byte, timeout time.Duration) (int, error) {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		n, err := conn.ReadPacket(buf)
		ch <- res{n, err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return 0, fmt.Errorf("timeout")
	case r := <-ch:
		return r.n, r.err
	}
}
