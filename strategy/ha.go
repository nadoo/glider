package strategy

import (
	"net"

	"github.com/nadoo/glider/proxy"
)

// high availability dialer
type haDialer struct{ *rrDialer }

// newHADialer .
func newHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &haDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (ha *haDialer) NextDialer(dstAddr string) proxy.Dialer {
	d := ha.fwdrs[ha.Index()]
	if !d.Enabled() || d.Priority() < ha.Priority() {
		d = ha.nextDialer(dstAddr)
	}
	return d
}

func (ha *haDialer) Dial(network, addr string) (net.Conn, error) {
	return ha.NextDialer(addr).Dial(network, addr)
}

func (ha *haDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return ha.NextDialer(addr).DialUDP(network, addr)
}
