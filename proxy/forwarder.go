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
	addr        string
	disabled    uint32
	failures    uint32
	MaxFailures uint32 //maxfailures to set to Disabled
	latency     int
}

// ForwarderFromURL parses `forward=` command line and returns a new forwarder
func ForwarderFromURL(s string) (f *Forwarder, err error) {
	ss := strings.Split(s, "#")
	var d Dialer
	for _, url := range strings.Split(ss[0], ",") {
		d, err = DialerFromURL(url, d)
		if err != nil {
			return nil, err
		}
	}

	f = NewForwarder(d)
	if len(ss) > 1 {
		err = f.parseOption(ss[1])
	}

	return f, err
}

// NewForwarder returns a new forwarder
func NewForwarder(dialer Dialer) *Forwarder {
	return &Forwarder{Dialer: dialer, addr: dialer.Addr()}
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
		log.F("[forwarder] %s, dials %s, error:%s", f.addr, addr, err)
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
