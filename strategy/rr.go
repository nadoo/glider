package strategy

import (
	"bytes"
	"io"
	"net"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// forwarder slice orderd by priority
type priSlice []*proxy.Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority() > p[j].Priority() }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// rrDialer is a round robin dialer
type rrDialer struct {
	fwdrs    priSlice
	index    int32
	priority uint32
	website  string
	interval int
}

// newRRDialer returns a new rrDialer
func newRRDialer(fwdrs []*proxy.Forwarder, website string, interval int) *rrDialer {
	rr := &rrDialer{fwdrs: fwdrs}
	sort.Sort(rr.fwdrs)

	rr.website = website
	if strings.IndexByte(rr.website, ':') == -1 {
		rr.website += ":80"
	}

	rr.interval = interval
	return rr
}

func (rr *rrDialer) Addr() string { return "STRATEGY" }

func (rr *rrDialer) Dial(network, addr string) (net.Conn, error) {
	return rr.nextDialer(addr).Dial(network, addr)
}

func (rr *rrDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return rr.nextDialer(addr).DialUDP(network, addr)
}

func (rr *rrDialer) NextDialer(dstAddr string) proxy.Dialer { return rr.nextDialer(dstAddr) }

func (rr *rrDialer) nextDialer(dstAddr string) *proxy.Forwarder {
	n := int32(len(rr.fwdrs))
	if n == 1 {
		return rr.fwdrs[0]
	}

	for _, fwder := range rr.fwdrs {
		if fwder.Enabled() {
			rr.SetPriority(fwder.Priority())
			break
		}
	}

	idx := rr.Index()
	if rr.fwdrs[idx].Priority() < rr.Priority() {
		idx = 0
	}

	found := false
	var i int32
	for i = 0; i < n; i++ {
		idx = (idx + 1) % n
		if rr.fwdrs[idx].Enabled() &&
			rr.fwdrs[idx].Priority() >= rr.Priority() {
			found = true
			rr.SetPriority(rr.fwdrs[idx].Priority())
			break
		}
	}

	if !found {
		rr.SetPriority(0)
		log.F("NO AVAILABLE PROXY FOUND! please check your network or proxy server settings.")
	}

	rr.SetIndex(idx)

	return rr.fwdrs[idx]
}

// Index returns the active forwarder's Index of rrDialer
func (rr *rrDialer) Index() int32 { return atomic.LoadInt32(&rr.index) }

// SetIndex sets the active forwarder's Index of rrDialer
func (rr *rrDialer) SetIndex(p int32) { atomic.StoreInt32(&rr.index, p) }

// Priority returns the active priority of rrDialer
func (rr *rrDialer) Priority() uint32 { return atomic.LoadUint32(&rr.priority) }

// SetPriority sets the active priority of rrDialer
func (rr *rrDialer) SetPriority(p uint32) { atomic.StoreUint32(&rr.priority, p) }

// Check implements the Checker interface
func (rr *rrDialer) Check() {
	for i := 0; i < len(rr.fwdrs); i++ {
		go rr.check(i)
	}
}

func (rr *rrDialer) check(i int) {
	f := rr.fwdrs[i]
	retry := 1
	buf := make([]byte, 4)

	for {
		time.Sleep(time.Duration(rr.interval) * time.Second * time.Duration(retry>>1))

		retry <<= 1
		if retry > 16 {
			retry = 16
		}

		if f.Priority() < rr.Priority() {
			continue
		}

		startTime := time.Now()
		rc, err := f.Dial("tcp", rr.website)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in dial: %s", f.Addr(), f.Priority(), rr.website, err)
			continue
		}

		rc.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

		_, err = io.ReadFull(rc, buf)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in read: %s", f.Addr(), f.Priority(), rr.website, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			f.Enable()
			retry = 2
			readTime := time.Since(startTime)
			f.SetLatency(int64(readTime))
			log.F("[check] %s(%d) -> %s, ENABLED. connect time: %s", f.Addr(), f.Priority(), rr.website, readTime.String())
		} else {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. server response: %s", f.Addr(), f.Priority(), rr.website, buf)
		}

		rc.Close()
	}
}
