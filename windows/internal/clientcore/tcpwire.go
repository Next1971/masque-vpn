package clientcore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/netip"
)

const (
	capsuleDatagram           uint64 = 0x00
	capsuleAddressAssign      uint64 = 0x01
	capsuleRouteAdvertisement uint64 = 0x03

	maxCapsuleLen = 65535
)

// tcpRoute is one IP range in a ROUTE_ADVERTISEMENT capsule. Protocol 0 means any.
type tcpRoute struct {
	Start      netip.Addr
	End        netip.Addr
	IPProtocol uint8
}

func appendVarint(b []byte, v uint64) []byte {
	switch {
	case v <= 63:
		return append(b, byte(v))
	case v <= 16383:
		return append(b, byte(v>>8)|0x40, byte(v))
	case v <= 1073741823:
		return append(b, byte(v>>24)|0x80, byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b,
			byte(v>>56)|0xc0, byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v),
		)
	}
}

func readVarint(r io.ByteReader) (uint64, error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	n := 1 << (first >> 6)
	v := uint64(first & 0x3f)
	for i := 1; i < n; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v = (v << 8) | uint64(b)
	}
	return v, nil
}

func appendCapsule(dst []byte, typ uint64, value []byte) []byte {
	dst = appendVarint(dst, typ)
	dst = appendVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func readCapsule(r *bufio.Reader) (uint64, []byte, error) {
	typ, err := readVarint(r)
	if err != nil {
		return 0, nil, err
	}
	n, err := readVarint(r)
	if err != nil {
		return 0, nil, err
	}
	if n > maxCapsuleLen {
		return 0, nil, fmt.Errorf("capsule %d is %d bytes", typ, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return typ, buf, nil
}

func appendAddressAssign(prefixes []netip.Prefix) ([]byte, error) {
	var payload []byte
	for i, p := range prefixes {
		if !p.IsValid() || p != p.Masked() {
			return nil, fmt.Errorf("assign prefix %d: %s", i, p)
		}
		payload = appendVarint(payload, 0)
		payload = appendAddress(payload, p)
	}
	return appendCapsule(nil, capsuleAddressAssign, payload), nil
}

func parseAddressAssign(value []byte) ([]netip.Prefix, error) {
	r := bytes.NewReader(value)
	var out []netip.Prefix
	for r.Len() > 0 {
		if _, err := readVarint(r); err != nil {
			return nil, fmt.Errorf("request id: %w", err)
		}
		p, err := parseAddress(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func appendAddress(dst []byte, p netip.Prefix) []byte {
	if p.Addr().Is4() {
		dst = append(dst, 4)
	} else {
		dst = append(dst, 6)
	}
	dst = append(dst, p.Addr().AsSlice()...)
	return append(dst, byte(p.Bits()))
}

func parseAddress(r io.ByteReader) (netip.Prefix, error) {
	ver, err := r.ReadByte()
	if err != nil {
		return netip.Prefix{}, err
	}
	var raw []byte
	switch ver {
	case 4:
		raw = make([]byte, 4)
	case 6:
		raw = make([]byte, 16)
	default:
		return netip.Prefix{}, fmt.Errorf("ip version %d", ver)
	}
	rd, ok := r.(io.Reader)
	if !ok {
		return netip.Prefix{}, errors.New("address reader")
	}
	if _, err := io.ReadFull(rd, raw); err != nil {
		return netip.Prefix{}, err
	}
	bits, err := r.ReadByte()
	if err != nil {
		return netip.Prefix{}, err
	}
	addr, ok := netip.AddrFromSlice(raw)
	if !ok {
		return netip.Prefix{}, errors.New("bad address")
	}
	if int(bits) > addr.BitLen() {
		return netip.Prefix{}, fmt.Errorf("prefix length %d", bits)
	}
	p := netip.PrefixFrom(addr, int(bits))
	if p != p.Masked() {
		return netip.Prefix{}, errors.New("host bits are not zero")
	}
	return p, nil
}

func appendRouteAdvertisement(routes []tcpRoute) ([]byte, error) {
	var payload []byte
	for i, rt := range routes {
		if !rt.Start.IsValid() || !rt.End.IsValid() || rt.Start.BitLen() != rt.End.BitLen() {
			return nil, fmt.Errorf("route %d: bad range", i)
		}
		if rt.Start.Compare(rt.End) > 0 {
			return nil, fmt.Errorf("route %d: start after end", i)
		}
		if rt.Start.Is4() {
			payload = append(payload, 4)
		} else {
			payload = append(payload, 6)
		}
		payload = append(payload, rt.Start.AsSlice()...)
		payload = append(payload, rt.End.AsSlice()...)
		payload = append(payload, rt.IPProtocol)
	}
	return appendCapsule(nil, capsuleRouteAdvertisement, payload), nil
}

func parseRouteAdvertisement(value []byte) ([]tcpRoute, error) {
	r := bytes.NewReader(value)
	var out []tcpRoute
	for r.Len() > 0 {
		ver, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		n := 4
		if ver == 6 {
			n = 16
		} else if ver != 4 {
			return nil, fmt.Errorf("ip version %d", ver)
		}
		startb := make([]byte, n)
		endb := make([]byte, n)
		if _, err := io.ReadFull(r, startb); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, endb); err != nil {
			return nil, err
		}
		proto, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		start, ok := netip.AddrFromSlice(startb)
		if !ok {
			return nil, errors.New("bad start")
		}
		end, ok := netip.AddrFromSlice(endb)
		if !ok {
			return nil, errors.New("bad end")
		}
		if start.Compare(end) > 0 {
			return nil, errors.New("start after end")
		}
		out = append(out, tcpRoute{Start: start, End: end, IPProtocol: proto})
	}
	return out, nil
}

func appendDatagram(pkt []byte) []byte {
	payload := appendVarint(nil, 0)
	payload = append(payload, pkt...)
	return appendCapsule(nil, capsuleDatagram, payload)
}

func parseDatagram(value []byte) ([]byte, error) {
	r := bytes.NewReader(value)
	id, err := readVarint(r)
	if err != nil {
		return nil, err
	}
	if id != 0 {
		return nil, nil
	}
	pkt, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(pkt) == 0 {
		return nil, errors.New("empty ip packet")
	}
	return pkt, nil
}
