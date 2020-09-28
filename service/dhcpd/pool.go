package dhcpd

import (
	"bytes"
	"errors"
	"math/rand"
	"net"
	"time"
)

type Pool struct {
	items []*item
}

func NewPool(lease time.Duration, ipStart, ipEnd net.IP) (*Pool, error) {
	items := make([]*item, 0)
	curip := ipStart.To4()
	for bytes.Compare(curip, ipEnd.To4()) <= 0 {
		ip := make([]byte, 4)
		copy(ip, curip)
		items = append(items, &item{lease: lease, ip: ip})
		curip[3]++
	}
	rand.Seed(time.Now().Unix())
	return &Pool{items: items}, nil
}

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
