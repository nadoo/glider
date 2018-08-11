package strategy

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
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

// rrDialer is the base struct of strategy dialer
type rrDialer struct {
	fwdrs []*proxy.Forwarder
	idx   int

	status sync.Map

	// for checking
	website  string
	interval int
}

// newRRDialer returns a new rrDialer
func newRRDialer(fwdrs []*proxy.Forwarder, website string, interval int) *rrDialer {
	rr := &rrDialer{fwdrs: fwdrs}

	rr.website = website
	rr.interval = interval

	for k := range fwdrs {
		rr.status.Store(k, true)
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

	found := false
	for i := 0; i < n; i++ {
		rr.idx = (rr.idx + 1) % n
		result, ok := rr.status.Load(rr.idx)
		if ok && result.(bool) {
			found = true
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
			rr.status.Store(idx, false)
			log.F("[check] %s -> %s, set to DISABLED. error in dial: %s", d.Addr(), rr.website, err)
			continue
		}

		c.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

		_, err = io.ReadFull(c, buf)
		if err != nil {
			rr.status.Store(idx, false)
			log.F("[check] %s -> %s, set to DISABLED. error in read: %s", d.Addr(), rr.website, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			rr.status.Store(idx, true)
			retry = 2
			dialTime := time.Since(startTime)
			log.F("[check] %s -> %s, set to ENABLED. connect time: %s", d.Addr(), rr.website, dialTime.String())
		} else {
			rr.status.Store(idx, false)
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

	result, ok := ha.status.Load(ha.idx)
	if ok && !result.(bool) {
		d = ha.nextDialer(addr)
	}

	return d.Dial(network, addr)
}

func (ha *haDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	d := ha.fwdrs[ha.idx]

	result, ok := ha.status.Load(ha.idx)
	if ok && !result.(bool) {
		d = ha.nextDialer(addr)
	}

	return d.DialUDP(network, addr)
}
