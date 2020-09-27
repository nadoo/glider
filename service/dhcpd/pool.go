package dhcpd

import (
	"bytes"
	"errors"
	"net"
	"time"
)

type Pool struct {
	lease time.Duration
	items []*item
}

func NewPool(lease time.Duration, ipStart, ipEnd net.IP) (*Pool, error) {
	items := make([]*item, 0)
	var currentIp = ipStart.To4()
	for bytes.Compare(currentIp, ipEnd.To4()) <= 0 {
		ip := make([]byte, 4)
		copy(ip, currentIp)
		i := &item{
			lease: lease,
			ip:    ip,
		}
		items = append(items, i)
		currentIp[3]++
	}
	return &Pool{lease: lease, items: items}, nil
}

func (p *Pool) AssignIP(mac net.HardwareAddr) (net.IP, error) {
	var ip net.IP
	for _, item := range p.items {
		if mac.String() == item.hardwareAddr.String() {
			return item.ip, nil
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
	ip           net.IP
	lease        time.Duration
	taken        bool
	hardwareAddr net.HardwareAddr
}

func (i *item) take(addr net.HardwareAddr) net.IP {
	if i.taken {
		return nil
	} else {
		i.taken = true
		go func() {
			timer := time.NewTimer(i.lease)
			<-timer.C
			i.hardwareAddr = nil
			i.taken = false
		}()
		i.hardwareAddr = addr
		return i.ip
	}
}
