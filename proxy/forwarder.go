package proxy

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/nadoo/glider/common/log"
)

// Forwarder is a forwarder
type Forwarder struct {
	Dialer
	Priority    int
	MaxFailures uint32 //maxfailures to set to Disabled

	addr     string
	disabled uint32
	failures uint32
	latency  int64
	intface  string // local interface or ip address
}

// ForwarderFromURL parses `forward=` command value and returns a new forwarder
func ForwarderFromURL(s, intface string) (f *Forwarder, err error) {
	f = &Forwarder{}

	ss := strings.Split(s, "#")
	if len(ss) > 1 {
		err = f.parseOption(ss[1])
	}

	iface := intface
	if f.intface != "" && f.intface != intface {
		iface = f.intface
	}

	var d Dialer
	d, err = NewDirect(iface)
	if err != nil {
		return nil, err
	}

	for _, url := range strings.Split(ss[0], ",") {
		d, err = DialerFromURL(url, d)
		if err != nil {
			return nil, err
		}
	}

	f.Dialer = d
	f.addr = d.Addr()

	return f, err
}

func (f *Forwarder) parseOption(option string) error {
	query, err := url.ParseQuery(option)
	if err != nil {
		return err
	}

	var priority uint64
	p := query.Get("priority")
	if p != "" {
		priority, err = strconv.ParseUint(p, 10, 32)
	}
	f.Priority = int(priority)

	f.intface = query.Get("interface")

	return err
}

// Addr .
func (f *Forwarder) Addr() string {
	return f.addr
}

// Dial .
func (f *Forwarder) Dial(network, addr string) (c net.Conn, err error) {
	c, err = f.Dialer.Dial(network, addr)
	if err != nil {
		atomic.AddUint32(&f.failures, 1)
		if f.Failures() >= f.MaxFailures {
			f.Disable()
			log.F("[forwarder] %s reaches maxfailures, set to disabled", f.addr)
		}
	}

	return c, err
}

// Failures returns the failuer count of forwarder
func (f *Forwarder) Failures() uint32 {
	return atomic.LoadUint32(&f.failures)
}

// Enable the forwarder
func (f *Forwarder) Enable() {
	atomic.StoreUint32(&f.disabled, 0)
	atomic.StoreUint32(&f.failures, 0)
}

// Disable the forwarder
func (f *Forwarder) Disable() {
	atomic.StoreUint32(&f.disabled, 1)
}

// Enabled returns the status of forwarder
func (f *Forwarder) Enabled() bool {
	return !isTrue(atomic.LoadUint32(&f.disabled))
}

func isTrue(n uint32) bool {
	return n&1 == 1
}

// Latency returns the latency of forwarder
func (f *Forwarder) Latency() int64 {
	return atomic.LoadInt64(&f.latency)
}

// SetLatency sets the latency of forwarder
func (f *Forwarder) SetLatency(l int64) {
	atomic.StoreInt64(&f.latency, l)
}
