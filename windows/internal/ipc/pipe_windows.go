//go:build windows

package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/Microsoft/go-winio"
)

// Listen starts the named pipe server. Authenticated local users may connect;
// the pipe is not reachable from the network.
func Listen() (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;AU)",
		MessageMode:        false,
		InputBufferSize:    1 << 20,
		OutputBufferSize:   1 << 20,
	}
	return winio.ListenPipe(PipeName, cfg)
}

func Dial() (net.Conn, error) {
	return winio.DialPipe(PipeName, nil)
}

type Hub struct {
	mu      sync.Mutex
	clients map[net.Conn]struct{}
	seq     atomic.Uint64
}

func NewHub() *Hub {
	return &Hub{clients: make(map[net.Conn]struct{})}
}

func (h *Hub) Add(c net.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Remove(c net.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(resp Response) {
	resp.Event = EventStatus
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	data = append(data, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		_, _ = c.Write(data)
	}
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

func ReadJSON(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}

func RoundTrip(cmd Request) (*Response, error) {
	c, err := Dial()
	if err != nil {
		return nil, fmt.Errorf("MASQUE service is not running (pipe %s): %w", PipeName, err)
	}
	defer c.Close()
	if cmd.ID == 0 {
		cmd.ID = 1
	}
	if err := WriteJSON(c, cmd); err != nil {
		return nil, err
	}
	var resp Response
	if err := ReadJSON(c, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "request failed"
		}
		return &resp, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}
