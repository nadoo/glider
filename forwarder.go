package main

import "net"

// Forwarder .
type Forwarder struct {
	addr    string
	cDialer Dialer
}

// NewForwarder .
func NewForwarder(addr string, cDialer Dialer) *Forwarder {
	if cDialer == nil {
		cDialer = Direct
	}

	return &Forwarder{addr: addr, cDialer: cDialer}
}

func (p *Forwarder) Addr() string { return p.addr }

func (p *Forwarder) Dial(network, addr string) (net.Conn, error) {
	return p.cDialer.Dial(network, addr)
}
