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
}

// NewDialer returns a new strategy dialer
func NewDialer(s []string, c *Config) proxy.Dialer {
	// global forwarders in xx.conf
	var fwdrs []*proxy.Forwarder
	for _, chain := range s {
		fwdr, err := proxy.ForwarderFromURL(chain)
		if err != nil {
			log.Fatal(err)
		}
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		return proxy.Direct
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
	default:
		log.F("not supported forward mode '%s', just use the first forward server.", c.Strategy)
		dialer = fwdrs[0]
	}

	return dialer
}

type forwarderSlice []*proxy.Forwarder

func (p forwarderSlice) Len() int           { return len(p) }
func (p forwarderSlice) Less(i, j int) bool { return p[i].Priority > p[j].Priority }
func (p forwarderSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// rrDialer is a rr dialer
type rrDialer struct {
	fwdrs    forwarderSlice
	idx      int
	priority int

	// for checking
	website  string
	interval int
}

// newRRDialer returns a new rrDialer
func newRRDialer(fs []*proxy.Forwarder, website string, interval int) *rrDialer {
	rr := &rrDialer{fwdrs: fs}
	rr.website = website
	rr.interval = interval

	sort.Sort(rr.fwdrs)
	rr.priority = rr.fwdrs[0].Priority

	for k := range rr.fwdrs {
		log.F("k: %d, %s, priority: %d", k, rr.fwdrs[k].Addr(), rr.fwdrs[k].Priority)
		go rr.checkDialer(k)
	}

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
		log.F("NO AVAILABLE PROXY FOUND! please check your network or proxy server settings.")
	}

	return rr.fwdrs[rr.idx]
}

func (rr *rrDialer) NextDialer(dstAddr string) proxy.Dialer {
	return rr.nextDialer(dstAddr)
}

// Check dialer
func (rr *rrDialer) checkDialer(idx int) {
	retry := 1
	buf := make([]byte, 4)

	if strings.IndexByte(rr.website, ':') == -1 {
		rr.website = rr.website + ":80"
	}

	d := rr.fwdrs[idx]

	for {
		time.Sleep(time.Duration(rr.interval) * time.Second * time.Duration(retry>>1))
		retry <<= 1

		if retry > 16 {
			retry = 16
		}

		startTime := time.Now()
		c, err := d.Dial("tcp", rr.website)
		if err != nil {
			rr.fwdrs[idx].Disable()
			log.F("[check] %s -> %s, set to DISABLED. error in dial: %s", d.Addr(), rr.website, err)
			continue
		}

		c.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

		_, err = io.ReadFull(c, buf)
		if err != nil {
			rr.fwdrs[idx].Disable()
			log.F("[check] %s -> %s, set to DISABLED. error in read: %s", d.Addr(), rr.website, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			rr.fwdrs[idx].Enable()
			retry = 2
			dialTime := time.Since(startTime)
			log.F("[check] %s -> %s, set to ENABLED. connect time: %s", d.Addr(), rr.website, dialTime.String())
		} else {
			rr.fwdrs[idx].Disable()
			log.F("[check] %s -> %s, set to DISABLED. server response: %s", d.Addr(), rr.website, buf)
		}

		c.Close()
	}
}

// high availability proxy
type haDialer struct {
	*rrDialer
}

// newHADialer .
func newHADialer(dialers []*proxy.Forwarder, webhost string, duration int) proxy.Dialer {
	return &haDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (ha *haDialer) Dial(network, addr string) (net.Conn, error) {
	d := ha.fwdrs[ha.idx]
	if !d.Enabled() {
		d = ha.nextDialer(addr)
	}
	return d.Dial(network, addr)
}

func (ha *haDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	d := ha.fwdrs[ha.idx]
	if !d.Enabled() {
		d = ha.nextDialer(addr)
	}
	return d.DialUDP(network, addr)
}
