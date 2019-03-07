package proxy

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/nadoo/glider/common/log"
)

// StatusHandler function will be called when the forwarder's status changed
type StatusHandler func(*Forwarder)

// Forwarder is a forwarder
type Forwarder struct {
	Dialer
	addr        string
	priority    uint32
	maxFailures uint32 // maxfailures to set to Disabled
	disabled    uint32
	failures    uint32
	latency     int64
	intface     string // local interface or ip address
	handlers    []StatusHandler
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

	// set forwarder to disabled by default
	// TODO: check here
	f.Disable()

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
	f.SetPriority(uint32(priority))

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
		f.IncFailures()
		if f.Failures() >= f.MaxFailures() && f.Enabled() {
			f.Disable()
			log.F("[forwarder] %s reaches maxfailures.", f.addr)
		}
	}

	return c, err
}

// Failures returns the failuer count of forwarder
func (f *Forwarder) Failures() uint32 {
	return atomic.LoadUint32(&f.failures)
}

// IncFailures increase the failuer count by 1
func (f *Forwarder) IncFailures() {
	atomic.AddUint32(&f.failures, 1)
}

// AddHandler adds a custom handler to handle the status change event
func (f *Forwarder) AddHandler(h StatusHandler) {
	f.handlers = append(f.handlers, h)
}

// Enable the forwarder
func (f *Forwarder) Enable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 1, 0) {
		for _, h := range f.handlers {
			h(f)
		}
	}
	atomic.StoreUint32(&f.failures, 0)
}

// Disable the forwarder
func (f *Forwarder) Disable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 0, 1) {
		for _, h := range f.handlers {
			h(f)
		}
	}
}

// Enabled returns the status of forwarder
func (f *Forwarder) Enabled() bool {
	return !isTrue(atomic.LoadUint32(&f.disabled))
}

func isTrue(n uint32) bool {
	return n&1 == 1
}

// Priority returns the priority of forwarder
func (f *Forwarder) Priority() uint32 {
	return atomic.LoadUint32(&f.priority)
}

// SetPriority sets the priority of forwarder
func (f *Forwarder) SetPriority(l uint32) {
	atomic.StoreUint32(&f.priority, l)
}

// MaxFailures returns the maxFailures of forwarder
func (f *Forwarder) MaxFailures() uint32 {
	return atomic.LoadUint32(&f.maxFailures)
}

// SetMaxFailures sets the maxFailures of forwarder
func (f *Forwarder) SetMaxFailures(l uint32) {
	atomic.StoreUint32(&f.maxFailures, l)
}

// Latency returns the latency of forwarder
func (f *Forwarder) Latency() int64 {
	return atomic.LoadInt64(&f.latency)
}

// SetLatency sets the latency of forwarder
func (f *Forwarder) SetLatency(l int64) {
	atomic.StoreInt64(&f.latency, l)
}
