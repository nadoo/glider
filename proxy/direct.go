package proxy

import (
	"net"

	"github.com/nadoo/glider/common/log"
)

// Direct proxy
type Direct struct {
	*net.Dialer
	addr net.Addr
}

// Default dialer
var Default = &Direct{Dialer: &net.Dialer{}}

// NewDirect returns a Direct dialer
func NewDirect(intface string) *Direct {
	d := &Direct{}
	dialer := &net.Dialer{}

	ip := net.ParseIP(intface)
	if ip == nil {
		iface, err := net.InterfaceByName(intface)
		if err != nil {
			return nil
		}

		addrs, err := iface.Addrs()
		if err != nil {
			d.addr = addrs[0]
		}
	}

	d.addr = &net.TCPAddr{
		IP:   ip,
		Port: 0,
	}

	dialer.LocalAddr = d.addr

	return d
}

// Addr returns forwarder's address
func (d *Direct) Addr() string { return "DIRECT" }

// Dial connects to the address addr on the network net
func (d *Direct) Dial(network, addr string) (net.Conn, error) {
	if network == "uot" {
		network = "udp"
	}

	c, err := d.Dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	return c, err
}

// DialUDP connects to the given address
func (d *Direct) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	pc, err := net.ListenPacket(network, d.Dialer.LocalAddr.String())
	if err != nil {
		log.F("ListenPacket error: %s", err)
		return nil, nil, err
	}

	uAddr, err := net.ResolveUDPAddr("udp", addr)
	return pc, uAddr, err
}

// NextDialer returns the next dialer
func (d *Direct) NextDialer(dstAddr string) Dialer { return d }
