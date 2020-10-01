package vless

import (
	"errors"
	"net"
	"net/url"

	"github.com/nadoo/glider/proxy"
)

// VLess struct.
type VLess struct {
	dialer proxy.Dialer
	addr   string
	uuid   [16]byte
}

func init() {
	proxy.RegisterDialer("vless", NewVLessDialer)
}

// NewVLess returns a vless proxy.
func NewVLess(s string, d proxy.Dialer) (*VLess, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	addr := u.Host
	uuid, err := StrToUUID(u.User.Username())
	if err != nil {
		return nil, err
	}

	p := &VLess{
		dialer: d,
		addr:   addr,
		uuid:   uuid,
	}

	return p, nil
}

// NewVLessDialer returns a vless proxy dialer.
func NewVLessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVLess(s, dialer)
}

// Addr returns forwarder's address.
func (s *VLess) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *VLess) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	return NewConn(rc, s.uuid, addr)
}

// DialUDP connects to the given address via the proxy.
func (s *VLess) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("vless client does not support udp now")
}
