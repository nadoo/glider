package main

import (
	"bytes"
	"io"
	"net"
	"strings"
	"time"
)

// NewStrategyDialer .
func NewStrategyDialer(strategy string, dialers []Dialer, website string, duration int) Dialer {
	var dialer Dialer
	if len(dialers) == 0 {
		dialer = Direct
	} else if len(dialers) == 1 {
		dialer = dialers[0]
	} else if len(dialers) > 1 {
		switch strategy {
		case "rr":
			dialer = newRRDialer(dialers, website, duration)
			logf("forward to remote servers in round robin mode.")
		case "ha":
			dialer = newHADialer(dialers, website, duration)
			logf("forward to remote servers in high availability mode.")
		default:
			logf("not supported forward mode '%s', just use the first forward server.", conf.Strategy)
			dialer = dialers[0]
		}
	}

	return dialer
}

// rrDialer
type rrDialer struct {
	dialers []Dialer
	idx     int

	status map[int]bool

	// for checking
	website  string
	duration int
}

// newRRDialer .
func newRRDialer(dialers []Dialer, website string, duration int) *rrDialer {
	rr := &rrDialer{dialers: dialers}

	rr.status = make(map[int]bool)
	rr.website = website
	rr.duration = duration

	for k := range dialers {
		rr.status[k] = true
		go rr.checkDialer(k)
	}

	return rr
}

func (rr *rrDialer) Addr() string { return "STRATEGY" }
func (rr *rrDialer) Dial(network, addr string) (net.Conn, error) {
	return rr.NextDialer(addr).Dial(network, addr)
}

func (rr *rrDialer) NextDialer(dstAddr string) Dialer {
	n := len(rr.dialers)
	if n == 1 {
		rr.idx = 0
	}

	found := false
	for i := 0; i < n; i++ {
		rr.idx = (rr.idx + 1) % n
		if rr.status[rr.idx] {
			found = true
			break
		}
	}

	if !found {
		logf("NO AVAILABLE PROXY FOUND! please check your network or proxy server settings.")
	}

	return rr.dialers[rr.idx]
}

// Check dialer
func (rr *rrDialer) checkDialer(idx int) {
	retry := 1
	buf := make([]byte, 4)

	if strings.IndexByte(rr.website, ':') == -1 {
		rr.website = rr.website + ":80"
	}

	d := rr.dialers[idx]

	for {
		time.Sleep(time.Duration(rr.duration) * time.Second * time.Duration(retry>>1))
		retry <<= 1

		if retry > 16 {
			retry = 16
		}

		startTime := time.Now()
		c, err := d.Dial("tcp", rr.website)
		if err != nil {
			rr.status[idx] = false
			logf("proxy-check %s -> %s, set to DISABLED. error in dial: %s", d.Addr(), rr.website, err)
			continue
		}

		c.Write([]byte("GET / HTTP/1.0"))
		c.Write([]byte("\r\n\r\n"))

		_, err = io.ReadFull(c, buf)
		if err != nil {
			rr.status[idx] = false
			logf("proxy-check %s -> %s, set to DISABLED. error in read: %s", d.Addr(), rr.website, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			rr.status[idx] = true
			retry = 2
			dialTime := time.Since(startTime)
			logf("proxy-check %s -> %s, set to ENABLED. connect time: %s", d.Addr(), rr.website, dialTime.String())
		} else {
			rr.status[idx] = false
			logf("proxy-check %s -> %s, set to DISABLED. server response: %s", d.Addr(), rr.website, buf)
		}

		c.Close()
	}
}

// high availability proxy
type haDialer struct {
	*rrDialer
}

// newHADialer .
func newHADialer(dialers []Dialer, webhost string, duration int) Dialer {
	return &haDialer{rrDialer: newRRDialer(dialers, webhost, duration)}
}

func (ha *haDialer) Dial(network, addr string) (net.Conn, error) {
	d := ha.dialers[ha.idx]

	if !ha.status[ha.idx] {
		d = ha.NextDialer(addr)
	}

	return d.Dial(network, addr)
}
