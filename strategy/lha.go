package strategy

import (
	"net"

	"github.com/nadoo/glider/proxy"
)

// latency based high availability dialer
type lhaDialer struct{ *rrDialer }

// newLHADialer .
func newLHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &lhaDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (lha *lhaDialer) nextDialer(dstAddr string) *proxy.Forwarder {
	idx := lha.Index()
	var lowest int64
	for i, fwder := range lha.fwdrs {
		if fwder.Enabled() {
			lha.SetPriority(fwder.Priority())
			lowest = fwder.Latency()
			idx = int32(i)
			break
		}
	}

	for i, fwder := range lha.fwdrs {
		if fwder.Enabled() && fwder.Priority() >= lha.Priority() && fwder.Latency() < lowest {
			lowest = fwder.Latency()
			idx = int32(i)
		}
	}

	lha.SetIndex(idx)
	return lha.fwdrs[idx]
}

func (lha *lhaDialer) Dial(network, addr string) (net.Conn, error) {
	return lha.nextDialer(addr).Dial(network, addr)
}

func (lha *lhaDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return lha.nextDialer(addr).DialUDP(network, addr)
}
