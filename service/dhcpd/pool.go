package dhcpd

import (
	"bytes"
	"errors"
	"math/rand"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Pool is a dhcp pool.
type Pool struct {
	items []*item
	mutex sync.RWMutex
	lease time.Duration
}

type item struct {
	ip     netip.Addr
	mac    net.HardwareAddr
	expire time.Time
}

// NewPool returns a new dhcp ip pool.
func NewPool(lease time.Duration, start, end netip.Addr) (*Pool, error) {
	if start.IsUnspecified() || end.IsUnspecified() || start.Is6() || end.Is6() {
		return nil, errors.New("start ip or end ip is wrong/nil, please check your config, note only ipv4 is supported")
	}

	s, e := ipv4ToNum(start), ipv4ToNum(end)
	if e < s {
		return nil, errors.New("start ip larger than end ip")
	}

	items := make([]*item, 0, e-s+1)
	for n := s; n <= e; n++ {
		items = append(items, &item{ip: numToIPv4(n)})
	}
	rand.Seed(time.Now().Unix())

	p := &Pool{items: items, lease: lease}
	go func() {
		for now := range time.Tick(time.Second) {
			p.mutex.Lock()
			for i := 0; i < len(items); i++ {
				if !items[i].expire.IsZero() && now.After(items[i].expire) {
					items[i].mac = nil
					items[i].expire = time.Time{}
				}
			}
			p.mutex.Unlock()
		}
	}()

	return p, nil
}

// LeaseIP leases an ip to mac from dhcp pool.
func (p *Pool) LeaseIP(mac net.HardwareAddr) (netip.Addr, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		if bytes.Equal(mac, item.mac) {
			return item.ip, nil
		}
	}

	idx := rand.Intn(len(p.items))
	for _, item := range p.items[idx:] {
		if item.mac == nil {
			item.mac = mac
			item.expire = time.Now().Add(p.lease)
			return item.ip, nil
		}
	}

	for _, item := range p.items {
		if item.mac == nil {
			item.mac = mac
			item.expire = time.Now().Add(p.lease)
			return item.ip, nil
		}
	}

	return netip.Addr{}, errors.New("no more ip can be leased")
}

// LeaseStaticIP leases static ip from pool according to the given mac.
func (p *Pool) LeaseStaticIP(mac net.HardwareAddr, ip netip.Addr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		if item.ip == ip {
			item.mac = mac
			item.expire = time.Time{}
		}
	}
}

// ReleaseIP releases ip from pool according to the given mac.
func (p *Pool) ReleaseIP(mac net.HardwareAddr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		// not static ip
		if !item.expire.IsZero() && bytes.Equal(mac, item.mac) {
			item.mac = nil
			item.expire = time.Time{}
		}
	}
}

func ipv4ToNum(addr netip.Addr) uint32 {
	ip := addr.AsSlice()
	n := uint32(ip[0])<<24 + uint32(ip[1])<<16
	return n + uint32(ip[2])<<8 + uint32(ip[3])
}

func numToIPv4(n uint32) netip.Addr {
	ip := [4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	return netip.AddrFrom4(ip)
}
