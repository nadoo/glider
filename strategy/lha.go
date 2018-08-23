package strategy

import (
	"github.com/nadoo/glider/proxy"
)

// latency based high availability dialer
type lhaDialer struct{ *rrDialer }

// newLHADialer .
func newLHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &lhaDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (lha *lhaDialer) NextDialer(dstAddr string) proxy.Dialer {
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
