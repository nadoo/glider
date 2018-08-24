package strategy

import (
	"hash/fnv"
	"net"

	"github.com/nadoo/glider/proxy"
)

// destination hashing dialer
type dhDialer struct{ *rrDialer }

// newDHDialer .
func newDHDialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &dhDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (dh *dhDialer) NextDialer(dstAddr string) proxy.Dialer {
	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	idx := fnv1a.Sum32() % uint32(len(dh.fwdrs))
	return dh.fwdrs[idx]
}

func (dh *dhDialer) Dial(network, addr string) (net.Conn, error) {
	return dh.NextDialer(addr).Dial(network, addr)
}

func (dh *dhDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return dh.NextDialer(addr).DialUDP(network, addr)
}
