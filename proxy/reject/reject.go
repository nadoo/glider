// Package reject implements a virtual proxy which always reject requests.
package reject

import (
	"errors"
	"net"

	"github.com/nadoo/glider/proxy"
)

// A Reject represents the base struct of a reject proxy.
type Reject struct{}

func init() {
	proxy.RegisterDialer("reject", NewRejectDialer)
}

// NewReject returns a reject proxy, reject://
func NewReject(s string, dialer proxy.Dialer) (*Reject, error) {
	return &Reject{}, nil
}

// NewRejectDialer returns a reject proxy dialer.
func NewRejectDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewReject(s, dialer)
}

// Addr returns forwarder's address.
func (s *Reject) Addr() string { return "REJECT" }

// NextDialer returns the next dialer.
func (s *Reject) NextDialer(dstAddr string) proxy.Dialer { return s }

// Dial connects to the address addr on the network net via the proxy.
func (s *Reject) Dial(network, addr string) (net.Conn, error) { return nil, errors.New("REJECT") }

// DialUDP connects to the given address via the proxy.
func (s *Reject) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("REJECT")
}
