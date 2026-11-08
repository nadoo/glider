package rule

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// StatusHandler function will be called when the forwarder's status changed.
type StatusHandler func(*Forwarder)

// Forwarder associates with a `-forward` command, usually a dialer or a chain of dialers.
type Forwarder struct {
	proxy.Dialer
	url         string
	addr        string
	priority    atomic.Uint32
	cascade     uint32
	weight      uint32
	retries     int
	retryDelay  time.Duration
	weightSet   atomic.Bool
	maxFailures atomic.Uint32 // maxfailures to set to Disabled
	disabled    atomic.Uint32
	failures    atomic.Uint32
	latency     atomic.Int64
	statusMu    sync.Mutex
	intface     string // local interface or ip address
	handlers    []StatusHandler
}

// ForwarderFromURL parses `forward=` command value and returns a new forwarder.
func ForwarderFromURL(s, intface string, dialTimeout, relayTimeout time.Duration) (f *Forwarder, err error) {
	f = &Forwarder{url: s, retryDelay: 200 * time.Millisecond}

	chain, option, found := strings.Cut(s, "#")
	if found {
		if strings.Contains(option, "#") {
			return nil, fmt.Errorf("forwarder options must not contain '#'")
		}
		if err = f.parseOption(option); err != nil {
			return nil, err
		}
	}

	iface := intface
	if f.intface != "" && f.intface != intface {
		iface = f.intface
	}

	var d proxy.Dialer
	d, err = proxy.NewDirect(iface, dialTimeout, relayTimeout)
	if err != nil {
		return nil, err
	}

	d, layerAddrs, err := chainFromURL(chain, d)
	if err != nil {
		return nil, err
	}

	f.Dialer = d

	var addrs []string
	for _, addr := range layerAddrs {
		if n := len(addrs); n == 0 || addrs[n-1] != addr {
			addrs = append(addrs, addr)
		}
	}
	f.addr = strings.Join(addrs, ",")

	// set forwarder to disabled by default
	f.Disable()

	return f, nil
}

// chainFromURL layers each comma-separated dialer in rawURL over base.
// Forwarder options after '#' are ignored. The returned slice holds the
// address of every layer in order.
func chainFromURL(rawURL string, base proxy.Dialer) (proxy.Dialer, []string, error) {
	if idx := strings.Index(rawURL, "#"); idx >= 0 {
		rawURL = rawURL[:idx]
	}

	var addrs []string
	for _, u := range strings.Split(rawURL, ",") {
		nd, err := proxy.DialerFromURL(u, base)
		if err != nil {
			return nil, nil, err
		}
		base = nd
		addrs = append(addrs, base.Addr())
	}
	return base, addrs, nil
}

// newForwarderFromConfig returns a forwarder for one configured forward URL.
func newForwarderFromConfig(s string, c *Strategy) (*Forwarder, error) {
	f, err := ForwarderFromURL(s, c.IntFace,
		time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
	if err != nil {
		return nil, err
	}
	f.SetMaxFailures(uint32(c.MaxFailures))
	return f, nil
}

// DirectForwarder returns a direct forwarder.
func DirectForwarder(intface string, dialTimeout, relayTimeout time.Duration) (*Forwarder, error) {
	d, err := proxy.NewDirect(intface, dialTimeout, relayTimeout)
	if err != nil {
		return nil, err
	}
	return &Forwarder{Dialer: d, addr: d.Addr()}, nil
}

func (f *Forwarder) parseOption(option string) error {
	query, err := url.ParseQuery(option)
	if err != nil {
		return err
	}

	for _, key := range []string{"attempts", "attemptdelay"} {
		if _, exists := query[key]; exists {
			// Block and delay budgets belong to a whole cascade level, so they
			// are only accepted in -cascade, never on a single forwarder.
			return fmt.Errorf("%s is supported only in -cascade", key)
		}
	}
	options, err := parseRetryOptions(query)
	if err != nil {
		return err
	}
	if options.Retries != nil {
		f.retries = *options.Retries
	}
	if options.RetryDelay != nil {
		f.retryDelay = time.Duration(*options.RetryDelay) * time.Millisecond
	}

	p := query.Get("priority")
	if p != "" {
		v, e := strconv.ParseUint(p, 10, 32)
		if e != nil {
			return e
		}
		f.SetPriority(uint32(v))
	}

	w := query.Get("weight")
	if w != "" {
		v, e := strconv.ParseUint(w, 10, 32)
		if e != nil {
			return e
		}
		f.SetWeight(uint32(v))
	}

	c := query.Get("cascade")
	if c != "" {
		v, e := strconv.ParseUint(c, 10, 32)
		if e != nil {
			return e
		}
		f.SetCascade(uint32(v))
	}

	f.intface = query.Get("interface")

	return nil
}

// Addr returns the forwarder's addr.
// NOTE: addr returns for chained dialers: dialer1Addr,dialer2Addr,...
func (f *Forwarder) Addr() string {
	return f.addr
}

// URL returns the forwarder's full url.
func (f *Forwarder) URL() string {
	return f.url
}

// Dial dials to addr and returns conn.
func (f *Forwarder) Dial(network, addr string) (c net.Conn, err error) {
	c, err = f.Dialer.Dial(network, addr)
	if err != nil {
		f.IncFailures()
	}
	return c, err
}

// Failures returns the failuer count of forwarder.
func (f *Forwarder) Failures() uint32 {
	return f.failures.Load()
}

// IncFailures increase the failuer count by 1.
func (f *Forwarder) IncFailures() {
	failures := f.failures.Add(1)
	if f.MaxFailures() == 0 {
		return
	}

	// log.F("[forwarder] %s(%d) recorded %d failures, maxfailures: %d", f.addr, f.Priority(), failures, f.MaxFailures())

	if failures == f.MaxFailures() && f.Enabled() {
		log.F("[forwarder] %s(%d) reaches maxfailures: %d", f.addr, f.Priority(), f.MaxFailures())
		f.Disable()
	}
}

// AddHandler adds a custom handler to handle the status change event.
func (f *Forwarder) AddHandler(h StatusHandler) {
	f.statusMu.Lock()
	defer f.statusMu.Unlock()
	f.handlers = append(f.handlers, h)
}

// Enable the forwarder.
func (f *Forwarder) Enable() {
	f.statusMu.Lock()
	defer f.statusMu.Unlock()
	if f.disabled.CompareAndSwap(1, 0) {
		for _, h := range f.handlers {
			h(f)
		}
	}
	f.failures.Store(0)
}

// Disable the forwarder.
func (f *Forwarder) Disable() {
	f.statusMu.Lock()
	defer f.statusMu.Unlock()
	if f.disabled.CompareAndSwap(0, 1) {
		for _, h := range f.handlers {
			h(f)
		}
	}
}

// Enabled returns the status of forwarder.
func (f *Forwarder) Enabled() bool {
	return !isTrue(f.disabled.Load())
}

func isTrue(n uint32) bool {
	return n&1 == 1
}

// Priority returns the priority of forwarder.
func (f *Forwarder) Priority() uint32 {
	return f.priority.Load()
}

// SetPriority sets the priority of forwarder.
func (f *Forwarder) SetPriority(l uint32) {
	f.priority.Store(l)
}

// Weight returns the weight of forwarder.
// Returns 1 if weight was not explicitly set (default), preserving explicit 0 for exclusion.
func (f *Forwarder) Weight() int64 {
	w := atomic.LoadUint32(&f.weight)
	if w == 0 && !f.weightSet.Load() {
		return 1
	}
	return int64(w)
}

// SetWeight sets the weight of forwarder.
func (f *Forwarder) SetWeight(w uint32) {
	f.weightSet.Store(true)
	atomic.StoreUint32(&f.weight, w)
}

// Cascade returns the cascade level of forwarder.
func (f *Forwarder) Cascade() uint32 {
	return atomic.LoadUint32(&f.cascade)
}

// SetCascade sets the cascade level of forwarder.
func (f *Forwarder) SetCascade(c uint32) {
	atomic.StoreUint32(&f.cascade, c)
}

// MaxFailures returns the maxFailures of forwarder.
func (f *Forwarder) MaxFailures() uint32 {
	return f.maxFailures.Load()
}

// SetMaxFailures sets the maxFailures of forwarder.
func (f *Forwarder) SetMaxFailures(l uint32) {
	f.maxFailures.Store(l)
}

// Latency returns the latency of forwarder.
func (f *Forwarder) Latency() int64 {
	return f.latency.Load()
}

// SetLatency sets the latency of forwarder.
func (f *Forwarder) SetLatency(l int64) {
	f.latency.Store(l)
}
