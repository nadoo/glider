package strategy

import (
	"bytes"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// Config of strategy
type Config struct {
	Strategy      string
	CheckWebSite  string
	CheckInterval int
	MaxFailures   int
	IntFace       string
}

// Checker is an interface of forwarder checker
type Checker interface {
	Check()
}

// NewDialer returns a new strategy dialer
func NewDialer(s []string, c *Config) proxy.Dialer {
	var fwdrs []*proxy.Forwarder
	for _, chain := range s {
		fwdr, err := proxy.ForwarderFromURL(chain, c.IntFace)
		if err != nil {
			log.Fatal(err)
		}
		fwdr.MaxFailures = uint32(c.MaxFailures)
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		d, err := proxy.NewDirect(c.IntFace)
		if err != nil {
			log.Fatal(err)
		}
		return d
	}

	if len(fwdrs) == 1 {
		return fwdrs[0]
	}

	var dialer proxy.Dialer
	switch c.Strategy {
	case "rr":
		dialer = newRRDialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in round robin mode.")
	case "ha":
		dialer = newHADialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in high availability mode.")
	case "lha":
		dialer = newLHADialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in latency based high availability mode.")
	default:
		log.F("not supported forward mode '%s', just use the first forward server.", c.Strategy)
		dialer = fwdrs[0]
	}

	return dialer
}

// slice orderd by priority
type priSlice []*proxy.Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority > p[j].Priority }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// rrDialer is a round robin dialer
// 1. find the highest priority which there's at least 1 dialer is enabled
// 2. choose a enabled dialer in that priority using round robin mode
type rrDialer struct {
	fwdrs priSlice

	// may have data races, but doesn't matter
	idx      int
	priority int

	// for checking
	website  string
	interval int
}

// newRRDialer returns a new rrDialer
func newRRDialer(fs []*proxy.Forwarder, website string, interval int) *rrDialer {
	rr := &rrDialer{fwdrs: fs}
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
	return rr.NextDialer(addr).Dial(network, addr)
}

func (rr *rrDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return rr.NextDialer(addr).DialUDP(network, addr)
}

func (rr *rrDialer) nextDialer(dstAddr string) *proxy.Forwarder {
	n := len(rr.fwdrs)
	if n == 1 {
		rr.idx = 0
	}

	for _, fwder := range rr.fwdrs {
		if fwder.Enabled() {
			rr.priority = fwder.Priority
			break
		}
	}

	if rr.fwdrs[rr.idx].Priority < rr.priority {
		rr.idx = 0
	}

	found := false
	for i := 0; i < n; i++ {
		rr.idx = (rr.idx + 1) % n
		if rr.fwdrs[rr.idx].Enabled() &&
			rr.fwdrs[rr.idx].Priority >= rr.priority {
			found = true
			rr.priority = rr.fwdrs[rr.idx].Priority
			break
		}
	}

	if !found {
		rr.priority = 0
		log.F("NO AVAILABLE PROXY FOUND! please check your network or proxy server settings.")
	}

	return rr.fwdrs[rr.idx]
}

func (rr *rrDialer) NextDialer(dstAddr string) proxy.Dialer {
	return rr.nextDialer(dstAddr)
}

// Check implements the Checker interface
func (rr *rrDialer) Check() {
	for _, f := range rr.fwdrs {
		go rr.checkDialer(f)
	}
}

// Check dialer
func (rr *rrDialer) checkDialer(f *proxy.Forwarder) {
	retry := 1
	buf := make([]byte, 4)

	for {
		time.Sleep(time.Duration(rr.interval) * time.Second * time.Duration(retry>>1))

		retry <<= 1
		if retry > 16 {
			retry = 16
		}

		// check forwarders whose priority not less than current priority only
		if f.Priority < rr.priority {
			// log.F("f.Priority:%d, rr.priority:%d", f.Priority, rr.priority)
			continue
		}

		startTime := time.Now()
		rc, err := f.Dial("tcp", rr.website)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in dial: %s", f.Addr(), f.Priority, rr.website, err)
			continue
		}

		rc.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

		_, err = io.ReadFull(rc, buf)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in read: %s", f.Addr(), f.Priority, rr.website, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			f.Enable()
			retry = 2
			readTime := time.Since(startTime)
			f.SetLatency(int64(readTime))
			log.F("[check] %s(%d) -> %s, ENABLED. connect time: %s", f.Addr(), f.Priority, rr.website, readTime.String())
		} else {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. server response: %s", f.Addr(), f.Priority, rr.website, buf)
		}

		rc.Close()
	}
}

// high availability forwarder
// 1. choose dialer whose priority is the highest
// 2. choose the first enabled dialer in that priority
type haDialer struct {
	*rrDialer
}

// newHADialer .
func newHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &haDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (ha *haDialer) nextDialer(dstAddr string) *proxy.Forwarder {
	d := ha.fwdrs[ha.idx]
	if !d.Enabled() {
		d = ha.nextDialer(dstAddr)
	}
	return d
}

func (ha *haDialer) Dial(network, addr string) (net.Conn, error) {
	d := ha.nextDialer(addr)
	return d.Dial(network, addr)
}

func (ha *haDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	d := ha.nextDialer(addr)
	return d.DialUDP(network, addr)
}

// latency based high availability forwarder
// 1. choose dialer whose priority is the highest
// 2. choose dialer with the lowest latency
type lhaDialer struct {
	*rrDialer
}

// newLHADialer .
func newLHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &lhaDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (lha *lhaDialer) nextDialer(dstAddr string) *proxy.Forwarder {
	var latency int64
	for i, fwder := range lha.fwdrs {
		if fwder.Enabled() {
			lha.priority = fwder.Priority
			latency = fwder.Latency()
			lha.idx = i
			break
		}
	}

	for i, fwder := range lha.fwdrs {
		if fwder.Enabled() && fwder.Priority >= lha.priority && fwder.Latency() < latency {
			latency = fwder.Latency()
			lha.idx = i
		}
	}

	return lha.fwdrs[lha.idx]
}

func (lha *lhaDialer) Dial(network, addr string) (net.Conn, error) {
	d := lha.nextDialer(addr)
	return d.Dial(network, addr)
}

func (lha *lhaDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	d := lha.nextDialer(addr)
	return d.DialUDP(network, addr)
}
