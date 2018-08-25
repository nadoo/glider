package strategy

import (
	"hash/fnv"

	"github.com/nadoo/glider/proxy"
)

func (d *Dialer) scheduleRR(dstAddr string) *proxy.Forwarder {
	d.mu.Lock()
	defer d.mu.Unlock()

	idx := d.IncIndex() % int32(len(d.valid))
	d.SetIndex(idx)
	return d.valid[idx]
}

func (d *Dialer) scheduleHA(dstAddr string) *proxy.Forwarder {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.valid[0]
}

func (d *Dialer) scheduleLHA(dstAddr string) *proxy.Forwarder {
	d.mu.Lock()
	defer d.mu.Unlock()

	fwdr := d.valid[0]
	lowest := fwdr.Latency()
	for _, f := range d.valid {
		if f.Latency() < lowest {
			lowest = f.Latency()
			fwdr = f
		}
	}

	return fwdr
}

func (d *Dialer) scheduleDH(dstAddr string) *proxy.Forwarder {
	d.mu.Lock()
	defer d.mu.Unlock()

	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	idx := fnv1a.Sum32() % uint32(len(d.valid))
	return d.valid[idx]
}
