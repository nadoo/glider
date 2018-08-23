package strategy

import (
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
