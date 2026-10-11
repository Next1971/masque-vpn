package clientcore

import (
	"fmt"
	"time"
)

func readPacketTimeout(conn packetSession, buf []byte, timeout time.Duration) (int, error) {
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
