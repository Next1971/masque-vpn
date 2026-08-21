// ippool.go is a thread-safe IPv4 address pool for client assignment (E1).
// pool_cidr assigns one /32 per client, excluding the server address,
// network address, and broadcast address.
package main

import (
	"fmt"
	"net/netip"
	"sync"
)

// IPPool allocates addresses from the specified CIDR, excluding reserved addresses.
type IPPool struct {
	mu        sync.Mutex
	free      []netip.Addr
	inUse     map[netip.Addr]bool
	prefixOf  map[netip.Addr]netip.Prefix
	sticky    map[string]netip.Addr // client id (cert CN) → last /32
	lease     map[string]uint64     // client id → current lease
	nextLease uint64
}

// NewIPPool builds a pool from poolCIDR, reserving serverAddr (the server tunnel address),
// the network address, and the range broadcast address.
func NewIPPool(poolCIDR string, serverAddr netip.Addr) (*IPPool, error) {
	prefix, err := netip.ParsePrefix(poolCIDR)
	if err != nil {
		return nil, fmt.Errorf("parse pool_cidr %q: %w", poolCIDR, err)
	}
	prefix = prefix.Masked()
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("only IPv4 pools supported, got %q", poolCIDR)
	}

	network := prefix.Addr()
	broadcast := lastIPOfPrefix(prefix)

	p := &IPPool{
		inUse:    make(map[netip.Addr]bool),
		prefixOf: make(map[netip.Addr]netip.Prefix),
		sticky:   make(map[string]netip.Addr),
		lease:    make(map[string]uint64),
	}

	addr := network.Next() // first host
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

func (p *IPPool) takeFree(addr netip.Addr) {
	for i, a := range p.free {
		if a == addr {
			p.free = append(p.free[:i], p.free[i+1:]...)
			return
		}
	}
}

// AllocateFor returns a /32. The same id (mTLS cert CN) is given the same
// address on reconnect so an Android TUN that already has that IP keeps working.
// lease must be passed to Release; a superseded session cannot free the address
// out from under the new one.
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
			pfx := netip.PrefixFrom(pref, 32)
			p.prefixOf[pref] = pfx
			return pfx, lease, nil
		}
	}

	if len(p.free) == 0 {
		return netip.Prefix{}, 0, fmt.Errorf("IP pool exhausted")
	}
	addr := p.free[len(p.free)-1]
	p.free = p.free[:len(p.free)-1]
	p.inUse[addr] = true
	pfx := netip.PrefixFrom(addr, 32)
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
	p.free = append(p.free, addr)
}

// Available returns the number of free addresses (for logs).
func (p *IPPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.free)
}
