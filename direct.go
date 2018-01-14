package main

import "net"

// direct proxy
type direct struct{}

// Direct proxy
var Direct = &direct{}

func (d *direct) Addr() string { return "DIRECT" }

func (d *direct) Dial(network, addr string) (net.Conn, error) {
	if network == "uot" {
		network = "udp"
	}

	c, err := net.Dial(network, addr)
	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}
	return c, err
}

func (d *direct) NextDialer(dstAddr string) Dialer { return d }
