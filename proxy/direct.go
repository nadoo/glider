package proxy

import (
	"errors"
	"net"
	"time"

	"github.com/nadoo/glider/log"
)

// Direct proxy.
type Direct struct {
	iface        *net.Interface // interface specified by user
	ip           net.IP
	dialTimeout  time.Duration
	relayTimeout time.Duration
}

func init() {
	RegisterDialer("direct", NewDirectDialer)
}

// NewDirect returns a Direct dialer.
func NewDirect(intface string, dialTimeout, relayTimeout time.Duration) (*Direct, error) {
	d := &Direct{dialTimeout: dialTimeout, relayTimeout: relayTimeout}

	if intface != "" {
		if ip := net.ParseIP(intface); ip != nil {
			d.ip = ip
		} else {
			iface, err := net.InterfaceByName(intface)
			if err != nil {
				return nil, errors.New(err.Error() + ": " + intface)
			}
			d.iface = iface
			if ips := d.IFaceIPs(); len(ips) > 0 {
				d.ip = ips[0]
			}
		}
	}

	return d, nil
}

// NewDirectDialer returns a direct dialer.
func NewDirectDialer(s string, d Dialer) (Dialer, error) {
	if d == nil {
		return NewDirect("", time.Duration(3)*time.Second, time.Duration(3)*time.Second)
	}
	return d, nil
}

// Addr returns forwarder's address.
func (d *Direct) Addr() string { return "DIRECT" }

// Dial connects to the address addr on the network net
func (d *Direct) Dial(network, addr string) (c net.Conn, err error) {
	if d.iface == nil || d.ip != nil {
		c, err = d.dial(network, addr, d.ip)
		if err == nil {
			return
		}
	}

	for _, ip := range d.IFaceIPs() {
		c, err = d.dial(network, addr, ip)
		if err == nil {
			d.ip = ip
			break
		}
	}

	// no ip available (so no dials made), maybe the interface link is down
	if c == nil && err == nil {
		err = errors.New("dial failed, maybe the interface link is down, please check it")
	}

	return c, err
}

func (d *Direct) dial(network, addr string, localIP net.IP) (net.Conn, error) {
	var la net.Addr
	switch network {
	case "tcp":
		la = &net.TCPAddr{IP: localIP}
	case "udp":
		la = &net.UDPAddr{IP: localIP}
	}

	dialer := &net.Dialer{LocalAddr: la, Timeout: d.dialTimeout}
	c, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	if d.relayTimeout > 0 {
		c.SetDeadline(time.Now().Add(d.relayTimeout))
	}

	return c, err
}

// DialUDP connects to the given address.
func (d *Direct) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	// TODO: support specifying local interface
	var la string
	if d.ip != nil {
		la = net.JoinHostPort(d.ip.String(), "0")
	}

	pc, err := net.ListenPacket(network, la)
	if err != nil {
		log.F("ListenPacket error: %s", err)
		return nil, nil, err
	}

	uAddr, err := net.ResolveUDPAddr("udp", addr)
	return pc, uAddr, err
}

// IFaceIPs returns ip addresses according to the specified interface.
func (d *Direct) IFaceIPs() (ips []net.IP) {
	ipnets, err := d.iface.Addrs()
	if err != nil {
		return
	}

	for _, ipnet := range ipnets {
		ips = append(ips, ipnet.(*net.IPNet).IP) //!ip.IsLinkLocalUnicast()
	}

	return
}
