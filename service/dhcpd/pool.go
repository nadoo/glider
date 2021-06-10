package dhcpd

import (
	"bytes"
	"errors"
	"math/rand"
	"net"
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
	ip     net.IP
	mac    net.HardwareAddr
	expire time.Time
}

// NewPool returns a new dhcp ip pool.
func NewPool(lease time.Duration, start, end net.IP) (*Pool, error) {
	if start == nil || end == nil {
		return nil, errors.New("start ip or end ip is wrong/nil, please check your config")
	}

	s, e := ip2num(start.To4()), ip2num(end.To4())
	if e < s {
		return nil, errors.New("start ip larger than end ip")
	}

	items := make([]*item, 0, e-s+1)
	for n := s; n <= e; n++ {
		items = append(items, &item{ip: num2ip(n)})
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
func (p *Pool) LeaseIP(mac net.HardwareAddr) (net.IP, error) {
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

	return nil, errors.New("no more ip can be leased")
}

// LeaseStaticIP leases static ip from pool according to the given mac.
func (p *Pool) LeaseStaticIP(mac net.HardwareAddr, ip net.IP) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		if item.ip.Equal(ip) {
			item.mac = mac
			item.expire = time.Now().Add(time.Hour * 24 * 365 * 50) // 50 years
		}
	}
}

// ReleaseIP releases ip from pool according to the given mac.
func (p *Pool) ReleaseIP(mac net.HardwareAddr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		if bytes.Equal(mac, item.mac) {
			item.mac = nil
			item.expire = time.Time{}
		}
	}
}

func ip2num(ip net.IP) uint32 {
	n := uint32(ip[0])<<24 + uint32(ip[1])<<16
	return n + uint32(ip[2])<<8 + uint32(ip[3])
}

func num2ip(n uint32) net.IP {
	return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
}
