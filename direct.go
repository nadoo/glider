package main

import "net"

// direct proxy
type direct struct {
}

// Direct proxy
var Direct = &direct{}

func (d *direct) Addr() string                  { return "127.0.0.1" }
func (d *direct) ListenAndServe()               { logf("base proxy ListenAndServe") }
func (d *direct) Serve(c net.Conn)              { logf("base proxy Serve") }
func (d *direct) CurrentProxy() Proxy           { return d }
func (d *direct) GetProxy(dstAddr string) Proxy { return d }
func (d *direct) NextProxy() Proxy              { return d }
func (d *direct) Enabled() bool                 { return true }
func (d *direct) SetEnable(enable bool)         {}

func (d *direct) Dial(network, addr string) (net.Conn, error) {
	c, err := net.Dial(network, addr)
	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}
	return c, err
}
