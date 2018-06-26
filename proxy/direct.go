package proxy

import (
	"net"

	"github.com/nadoo/glider/common/log"
)

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
	if err != nil {
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	return c, err
}

// DialUDP connects to the given address via the proxy
func (d *direct) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	pc, err := net.ListenPacket(network, "")
	if err != nil {
		log.F("ListenPacket error: %s", err)
		return nil, nil, err
	}

	uAddr, err := net.ResolveUDPAddr("udp", addr)
	return pc, uAddr, err
}

func (d *direct) NextDialer(dstAddr string) Dialer { return d }
