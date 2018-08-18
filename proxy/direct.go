package proxy

import (
	"net"

	"github.com/nadoo/glider/common/log"
)

// Direct proxy
type Direct struct {
	iface *net.Interface
	ip    net.IP
}

// Default dialer
var Default = &Direct{}

// NewDirect returns a Direct dialer
func NewDirect(intface string) (*Direct, error) {
	if intface == "" {
		return &Direct{}, nil
	}

	ip := net.ParseIP(intface)
	if ip != nil {
		return &Direct{ip: ip}, nil
	}

	iface, err := net.InterfaceByName(intface)
	if err != nil {
		return nil, err
	}

	return &Direct{iface: iface}, nil
}

// Addr returns forwarder's address
func (d *Direct) Addr() string { return "DIRECT" }

// Dial connects to the address addr on the network net
func (d *Direct) Dial(network, addr string) (c net.Conn, err error) {
	for _, ip := range d.LocalIPs() {
		c, err = d.dial(network, addr, ip)
		// log.F("dial %s using ip: %s", addr, ip)
		if err == nil {
			break
		}
	}
	return
}

func (d *Direct) dial(network, addr string, localIP net.IP) (net.Conn, error) {
	if network == "uot" {
		network = "udp"
	}

	var localAddr net.Addr
	switch network {
	case "tcp":
		localAddr = &net.TCPAddr{IP: localIP}
	case "udp":
		localAddr = &net.UDPAddr{IP: localIP}
	}

	dialer := &net.Dialer{LocalAddr: localAddr}
	c, err := dialer.Dial(network, addr)
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
	// TODO: support specifying local interface
	pc, err := net.ListenPacket(network, "")
	if err != nil {
		log.F("ListenPacket error: %s", err)
		return nil, nil, err
	}

	uAddr, err := net.ResolveUDPAddr("udp", addr)
	return pc, uAddr, err
}

// NextDialer returns the next dialer
func (d *Direct) NextDialer(dstAddr string) Dialer { return d }

// LocalIPs returns ip addresses according to the specified interface
func (d *Direct) LocalIPs() (ips []net.IP) {
	if d.ip != nil {
		ips = []net.IP{d.ip}
		return
	}

	if d.iface == nil {
		ips = []net.IP{nil}
		return
	}

	ipnets, err := d.iface.Addrs()
	if err != nil {
		return
	}

	for _, ipnet := range ipnets {
		ips = append(ips, ipnet.(*net.IPNet).IP)
	}

	return
}
