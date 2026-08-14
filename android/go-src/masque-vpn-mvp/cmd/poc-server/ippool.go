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
	mu       sync.Mutex
	free     []netip.Addr        // available addresses (stack)
	inUse    map[netip.Addr]bool // in use
	prefixOf map[netip.Addr]netip.Prefix
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

// Allocate returns a free address as a /32 prefix. It errors if the pool is exhausted.
func (p *IPPool) Allocate() (netip.Prefix, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.free) == 0 {
		return netip.Prefix{}, fmt.Errorf("IP pool exhausted")
	}
	addr := p.free[len(p.free)-1]
	p.free = p.free[:len(p.free)-1]
	p.inUse[addr] = true
	pfx := netip.PrefixFrom(addr, 32)
	p.prefixOf[addr] = pfx
	return pfx, nil
}

// Release returns an address to the pool.
func (p *IPPool) Release(pfx netip.Prefix) {
	addr := pfx.Addr()
	p.mu.Lock()
	defer p.mu.Unlock()
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
