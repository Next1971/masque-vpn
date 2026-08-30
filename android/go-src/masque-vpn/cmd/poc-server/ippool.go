// ippool.go is a thread-safe address pool for client assignment.
// IPv4 pools enumerate host addresses (excluding network, broadcast, server).
// IPv6 pools walk sequentially (a /64 cannot be pre-listed); assignment is /128.
package main

import (
	"fmt"
	"net/netip"
	"sync"
)

const maxIPv6Search = 65536

// IPPool allocates addresses from the specified CIDR, excluding reserved addresses.
type IPPool struct {
	mu        sync.Mutex
	prefix    netip.Prefix
	server    netip.Addr
	is4       bool
	free      []netip.Addr // IPv4 (and tiny IPv6) pre-listed hosts
	next      netip.Addr   // IPv6 sequential cursor
	inUse     map[netip.Addr]bool
	prefixOf  map[netip.Addr]netip.Prefix
	sticky    map[string]netip.Addr // client id (cert CN) → last host address
	lease     map[string]uint64     // client id → current lease
	nextLease uint64
}

// NewIPPool builds a pool from poolCIDR, reserving serverAddr (the server tunnel address).
func NewIPPool(poolCIDR string, serverAddr netip.Addr) (*IPPool, error) {
	prefix, err := netip.ParsePrefix(poolCIDR)
	if err != nil {
		return nil, fmt.Errorf("parse pool_cidr %q: %w", poolCIDR, err)
	}
	prefix = prefix.Masked()
	if prefix.Addr().Is4() != serverAddr.Is4() {
		return nil, fmt.Errorf("pool %q family does not match server address %s", poolCIDR, serverAddr)
	}
	if !prefix.Contains(serverAddr) {
		return nil, fmt.Errorf("server address %s is outside pool %s", serverAddr, prefix)
	}

	p := &IPPool{
		prefix:   prefix,
		server:   serverAddr,
		is4:      prefix.Addr().Is4(),
		inUse:    make(map[netip.Addr]bool),
		prefixOf: make(map[netip.Addr]netip.Prefix),
		sticky:   make(map[string]netip.Addr),
		lease:    make(map[string]uint64),
	}

	if p.is4 {
		network := prefix.Addr()
		broadcast := lastIPOfPrefix(prefix)
		addr := network.Next()
		for addr.Is4() && addr.Less(broadcast) {
			if addr != serverAddr {
				p.free = append(p.free, addr)
			}
			addr = addr.Next()
		}
		if len(p.free) == 0 {
			return nil, fmt.Errorf("pool %q has no assignable addresses", poolCIDR)
		}
		return p, nil
	}

	p.next = firstIPv6Candidate(prefix, serverAddr)
	if !p.next.IsValid() {
		return nil, fmt.Errorf("pool %q has no assignable addresses", poolCIDR)
	}
	return p, nil
}

func firstIPv6Candidate(prefix netip.Prefix, server netip.Addr) netip.Addr {
	addr := prefix.Addr().Next() // skip subnet-router anycast (::0)
	if addr == server {
		addr = addr.Next()
	}
	if !prefix.Contains(addr) {
		return netip.Addr{}
	}
	return addr
}

func (p *IPPool) takeFree(addr netip.Addr) {
	for i, a := range p.free {
		if a == addr {
			p.free = append(p.free[:i], p.free[i+1:]...)
			return
		}
	}
}

func (p *IPPool) hostPrefix(addr netip.Addr) netip.Prefix {
	bits := 128
	if p.is4 {
		bits = 32
	}
	return netip.PrefixFrom(addr, bits)
}

func (p *IPPool) allocFreshLocked() (netip.Addr, error) {
	if p.is4 {
		if len(p.free) == 0 {
			return netip.Addr{}, fmt.Errorf("IP pool exhausted")
		}
		addr := p.free[len(p.free)-1]
		p.free = p.free[:len(p.free)-1]
		return addr, nil
	}
	start := p.next
	for i := 0; i < maxIPv6Search; i++ {
		addr := p.next
		if !p.prefix.Contains(addr) {
			p.next = firstIPv6Candidate(p.prefix, p.server)
			if !p.next.IsValid() {
				break
			}
			addr = p.next
		}
		p.next = addr.Next()
		if addr == p.server || p.inUse[addr] {
			if p.next == start || (i > 0 && addr == start) {
				break
			}
			continue
		}
		return addr, nil
	}
	return netip.Addr{}, fmt.Errorf("IP pool exhausted")
}

// AllocateFor returns a host prefix (/32 or /128). The same id (mTLS cert CN)
// is given the same address on reconnect. lease must be passed to Release.
func (p *IPPool) AllocateFor(id string) (netip.Prefix, uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextLease++
	lease := p.nextLease

	if id != "" {
		p.lease[id] = lease
		if pref, ok := p.sticky[id]; ok {
			p.takeFree(pref)
			p.inUse[pref] = true
			pfx := p.hostPrefix(pref)
			p.prefixOf[pref] = pfx
			return pfx, lease, nil
		}
	}

	addr, err := p.allocFreshLocked()
	if err != nil {
		return netip.Prefix{}, 0, err
	}
	p.inUse[addr] = true
	pfx := p.hostPrefix(addr)
	p.prefixOf[addr] = pfx
	if id != "" {
		p.sticky[id] = addr
	}
	return pfx, lease, nil
}

// Release returns an address to the free list but keeps the sticky mapping.
func (p *IPPool) Release(id string, lease uint64, pfx netip.Prefix) {
	addr := pfx.Addr()
	p.mu.Lock()
	defer p.mu.Unlock()
	if id != "" && p.lease[id] != lease {
		return
	}
	if id != "" {
		delete(p.lease, id)
	}
	if !p.inUse[addr] {
		return
	}
	delete(p.inUse, addr)
	delete(p.prefixOf, addr)
	if p.is4 {
		p.free = append(p.free, addr)
	}
}

// Available returns the number of free addresses (IPv4). IPv6 reports 1 if
// the cursor can still walk, else 0 — not a precise remaining count.
func (p *IPPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.is4 {
		return len(p.free)
	}
	if p.next.IsValid() && p.prefix.Contains(p.next) {
		return 1
	}
	return 0
}
