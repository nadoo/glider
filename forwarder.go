package main

import "net"

// Forwarder struct
type Forwarder struct {
	addr    string
	cDialer Dialer
}

// NewForwarder returns a base forwarder
func NewForwarder(addr string, cDialer Dialer) *Forwarder {
	if cDialer == nil {
		cDialer = Direct
	}

	return &Forwarder{addr: addr, cDialer: cDialer}
}

// Addr returns forwarder's address
func (p *Forwarder) Addr() string { return p.addr }

// Dial to remote addr via cDialer
func (p *Forwarder) Dial(network, addr string) (net.Conn, error) {
	return p.cDialer.Dial(network, addr)
}

// NextDialer returns the next cDialer
func (p *Forwarder) NextDialer(dstAddr string) Dialer {
	return p.cDialer
}
