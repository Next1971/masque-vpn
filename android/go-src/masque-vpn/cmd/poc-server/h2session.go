package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
)

// tcpSession is one HTTP/2 CONNECT-IP stream. IP packets are DATAGRAM capsules.
type tcpSession struct {
	br    *bufio.Reader
	w     io.Writer
	flush func() error

	mu       sync.Mutex
	writeErr error

	packets chan []byte

	once sync.Once
	err  error
	done chan struct{}

	closeIO func()
}

func newTCPSession(r io.Reader, w io.Writer, flush func() error, closeIO func()) *tcpSession {
	s := &tcpSession{
		br:      bufio.NewReaderSize(r, 64*1024),
		w:       w,
		flush:   flush,
		packets: make(chan []byte, 32),
		done:    make(chan struct{}),
		closeIO: closeIO,
	}
	go s.readLoop()
	return s
}

func (s *tcpSession) fail(err error) {
	s.once.Do(func() {
		if err == nil {
			err = io.EOF
		}
		s.err = err
		close(s.done)
	})
}

func (s *tcpSession) Close() error {
	s.fail(net.ErrClosed)
	if s.closeIO != nil {
		s.closeIO()
	}
	return nil
}

func (s *tcpSession) readLoop() {
	for {
		typ, val, err := readCapsule(s.br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				s.fail(io.EOF)
			} else {
				s.fail(err)
			}
			return
		}
		if typ != capsuleDatagram {
			continue
		}
		pkt, err := parseDatagram(val)
		if err != nil {
			s.fail(err)
			return
		}
		if pkt == nil {
			continue
		}
		select {
		case s.packets <- pkt:
		case <-s.done:
			return
		}
	}
}

func (s *tcpSession) writeRaw(frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		if s.err != nil {
			return s.err
		}
		return net.ErrClosed
	default:
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	if _, err := s.w.Write(frame); err != nil {
		s.writeErr = err
		return err
	}
	if s.flush != nil {
		if err := s.flush(); err != nil {
			s.writeErr = err
			return err
		}
	}
	return nil
}

func (s *tcpSession) AssignAddresses(prefixes []netip.Prefix) error {
	frame, err := appendAddressAssign(prefixes)
	if err != nil {
		return err
	}
	return s.writeRaw(frame)
}

func (s *tcpSession) AdvertiseRoutes(routes []tcpRoute) error {
	frame, err := appendRouteAdvertisement(routes)
	if err != nil {
		return err
	}
	return s.writeRaw(frame)
}

func (s *tcpSession) WritePacket(pkt []byte) ([]byte, error) {
	if len(pkt) == 0 {
		return nil, errors.New("empty packet")
	}
	if err := s.writeRaw(appendDatagram(pkt)); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *tcpSession) ReadPacket(buf []byte) (int, error) {
	select {
	case p := <-s.packets:
		if len(p) > len(buf) {
			return 0, io.ErrShortBuffer
		}
		return copy(buf, p), nil
	case <-s.done:
		if s.err != nil {
			return 0, s.err
		}
		return 0, io.EOF
	}
}
