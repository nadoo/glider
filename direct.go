package main

import "net"

// direct proxy
type direct struct {
	Proxy
}

// Direct proxy
var Direct = &direct{Proxy: &proxy{addr: "127.0.0.1"}}

// Direct proxy always enabled
func (d *direct) Enabled() bool {
	return true
}

func (d *direct) Dial(network, addr string) (net.Conn, error) {
	c, err := net.Dial(network, addr)
	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}
	return c, err
}
