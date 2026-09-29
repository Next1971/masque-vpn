package clientcore

import (
	"fmt"
	"time"

	"net/netip"
)

// ProbeGateway checks that IP packets actually cross the CONNECT-IP session.
// It sends one ICMP echo to the server tunnel address (.1 in the client's /24)
// and waits for the echo reply. Call it before any other ReadPacket loop.
// On failure the session is closed so a stuck read cannot steal later packets.
func (s *Session) ProbeGateway(timeout time.Duration) error {
	if s == nil || s.ipconn == nil {
		return fmt.Errorf("probe: no session")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	v4, _ := SplitAssigned(s.AssignedPrefixes)
	if !v4.IsValid() {
		s.Close()
		return fmt.Errorf("probe: no IPv4 address")
	}
	gw := tunnelGateway4(v4.Addr())
	pkt := icmpEchoRequest(v4.Addr(), gw, 1)
	if _, err := s.ipconn.WritePacket(pkt); err != nil {
		s.Close()
		return fmt.Errorf("probe write: %w", err)
	}

	buf := make([]byte, 1500)
	deadline := time.Now().Add(timeout)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			s.Close()
			return fmt.Errorf("probe: no echo reply from %s", gw)
		}
		n, err := readPacketTimeout(s.ipconn, buf, remain)
		if err != nil {
			s.Close()
			return fmt.Errorf("probe read: %w", err)
		}
		if icmpEchoReplyFrom(buf[:n], gw) {
			return nil
		}
	}
}

// tunnelGateway4 is the server address on the IPv4 tunnel. The pool reserves
// the first host of the /24 that contains the client (/32) assignment.
func tunnelGateway4(client netip.Addr) netip.Addr {
	p := netip.PrefixFrom(client, 24).Masked()
	b := p.Addr().As4()
	b[3]++
	return netip.AddrFrom4(b)
}

func icmpEchoRequest(src, dst netip.Addr, seq uint16) []byte {
	const ipHdrLen = 20
	const icmpLen = 8
	total := ipHdrLen + icmpLen
	b := make([]byte, total)
	b[0] = 0x45
	b[2] = byte(total >> 8)
	b[3] = byte(total)
	b[6], b[7] = 0x40, 0x00
	b[8] = 64
	b[9] = 1
	s4 := src.As4()
	d4 := dst.As4()
	copy(b[12:16], s4[:])
	copy(b[16:20], d4[:])
	putChecksum(b[:ipHdrLen], 10)
	icmp := b[ipHdrLen:]
	icmp[0] = 8
	icmp[4], icmp[5] = 0x12, 0x34
	icmp[6] = byte(seq >> 8)
	icmp[7] = byte(seq)
	putChecksum(icmp, 2)
	return b
}

func icmpEchoReplyFrom(pkt []byte, from netip.Addr) bool {
	if len(pkt) < 28 || pkt[0]>>4 != 4 || pkt[9] != 1 {
		return false
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl+8 {
		return false
	}
	src := netip.AddrFrom4([4]byte{pkt[12], pkt[13], pkt[14], pkt[15]})
	return src == from && pkt[ihl] == 0
}

func putChecksum(b []byte, offset int) {
	b[offset] = 0
	b[offset+1] = 0
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	cs := ^uint16(sum)
	b[offset] = byte(cs >> 8)
	b[offset+1] = byte(cs)
}
