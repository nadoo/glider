package dhcpd

import (
	"bytes"
	"errors"
	"math/rand"
	"net"
	"time"
)

// Pool is a dhcp pool.
type Pool struct {
	items []*item
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
		items = append(items, &item{lease: lease, ip: num2ip(n)})
	}
	rand.Seed(time.Now().Unix())
	return &Pool{items: items}, nil
}

// AssignIP assigns an ip to mac from dhco pool.
func (p *Pool) AssignIP(mac net.HardwareAddr) (net.IP, error) {
	var ip net.IP
	for _, item := range p.items {
		if bytes.Equal(mac, item.mac) {
			return item.ip, nil
		}
	}

	idx := rand.Intn(len(p.items))
	for _, item := range p.items[idx:] {
		if ip = item.take(mac); ip != nil {
			return ip, nil
		}
	}

	for _, item := range p.items {
		if ip = item.take(mac); ip != nil {
			return ip, nil
		}
	}
	return nil, errors.New("no more ip can be assigned")
}

type item struct {
	taken bool
	ip    net.IP
	lease time.Duration
	mac   net.HardwareAddr
}

func (i *item) take(addr net.HardwareAddr) net.IP {
	if !i.taken {
		i.taken = true
		go func() {
			timer := time.NewTimer(i.lease)
			<-timer.C
			i.mac = nil
			i.taken = false
		}()
		i.mac = addr
		return i.ip
	}
	return nil
}

func ip2num(ip net.IP) uint32 {
	n := uint32(ip[0])<<24 + uint32(ip[1])<<16
	return n + uint32(ip[2])<<8 + uint32(ip[3])
}

func num2ip(n uint32) net.IP {
	return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
}
