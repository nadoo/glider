package rule

import (
	"errors"
	"hash/fnv"
	"math/rand/v2"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

var errNoAvailableForwarder = errors.New("no available forwarder")

// errorDialer implements proxy.Dialer and proxy.UDPDialer with a stored error.
// It is returned on empty selection so callers can safely call Addr() even on failure.
type errorDialer struct{ err error }

func (e *errorDialer) Addr() string                                         { return "error" }
func (e *errorDialer) Dial(network, addr string) (net.Conn, error)          { return nil, e.err }
func (e *errorDialer) DialUDP(network, addr string) (net.PacketConn, error) { return nil, e.err }

// FwdrGroup is a forwarder group.
type FwdrGroup struct {
	name     string
	config   *Strategy
	fwdrs    []*Forwarder
	avail    []*Forwarder // available forwarders
	mu       sync.RWMutex
	index    atomic.Uint32
	priority atomic.Uint32
	next     func(addr string) *Forwarder
}

// NewFwdrGroup returns a new forward group.
func NewFwdrGroup(rulePath string, s []string, c *Strategy) *FwdrGroup {
	var fwdrs []*Forwarder
	for _, chain := range s {
		fwdr, err := newForwarderFromConfig(chain, c)
		if err != nil {
			log.Fatal(err)
		}
		if fwdr.Cascade() > 0 {
			log.Fatal("cascade is supported only by main forwarders")
		}
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		// direct forwarder
		direct, err := DirectForwarder(c.IntFace,
			time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		fwdrs = append(fwdrs, direct)
		// The direct fallback always uses round robin. Copy the strategy so
		// the caller's config is not rewritten.
		directCfg := *c
		directCfg.Strategy = "rr"
		c = &directCfg
	}

	name := strings.TrimSuffix(filepath.Base(rulePath), filepath.Ext(rulePath))
	return newFwdrGroup(name, fwdrs, c)
}

// newFwdrGroup returns a new FwdrGroup.
func newFwdrGroup(name string, fwdrs []*Forwarder, c *Strategy) *FwdrGroup {
	p := &FwdrGroup{name: name, fwdrs: fwdrs, config: c}
	slices.SortFunc(p.fwdrs, func(a, b *Forwarder) int {
		switch {
		case a.Priority() > b.Priority():
			return -1
		case a.Priority() < b.Priority():
			return 1
		default:
			return 0
		}
	})

	p.init()

	// default scheduler
	p.next = p.scheduleRR

	// if there're more than 1 forwarders, we care about the strategy.
	if count := len(fwdrs); count > 1 {
		switch c.Strategy {
		case "rr":
			p.next = p.scheduleRR
			log.F("[strategy] %s: %d forwarders forward in round robin mode.", name, count)
		case "ha":
			p.next = p.scheduleHA
			log.F("[strategy] %s: %d forwarders forward in high availability mode.", name, count)
		case "lha":
			p.next = p.scheduleLHA
			log.F("[strategy] %s: %d forwarders forward in latency based high availability mode.", name, count)
		case "dh":
			p.next = p.scheduleDH
			log.F("[strategy] %s: %d forwarders forward in destination hashing mode.", name, count)
		case "pr":
			p.next = p.schedulePR
			log.F("[strategy] %s: %d forwarders forward in probability random mode.", name, count)
		case "prs":
			p.next = p.schedulePR
			log.F("[strategy] %s: %d forwarders forward in probability random (strict) mode.", name, count)
		default:
			p.next = p.scheduleRR
			log.F("[strategy] %s: not supported forward mode '%s', use round robin mode for %d forwarders.", name, c.Strategy, count)
		}
	}

	for _, f := range fwdrs {
		f.AddHandler(p.onStatusChanged)
	}

	return p
}

// Dial connects to the address addr on the network net.
func (p *FwdrGroup) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	f := p.NextDialer(addr)
	if f == nil {
		return nil, &errorDialer{errNoAvailableForwarder}, errNoAvailableForwarder
	}
	log.F("[dial] %s -> %s via %s (strategy=%s weight=%d)", p.name, addr, f.Addr(), p.config.Strategy, f.Weight())
	c, err := f.Dial(network, addr)
	return c, f, err
}

// DialUDP connects to the given address.
func (p *FwdrGroup) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	f := p.NextDialer(addr)
	if f == nil {
		return nil, &errorDialer{errNoAvailableForwarder}, errNoAvailableForwarder
	}
	log.F("[dial] %s UDP -> %s via %s (strategy=%s weight=%d)", p.name, addr, f.Addr(), p.config.Strategy, f.Weight())
	pc, err = f.DialUDP(network, addr)
	return pc, f, err
}

// NextDialer returns the next forwarder. It returns nil for the pr/prs
// strategies when no forwarder is available.
func (p *FwdrGroup) NextDialer(dstAddr string) *Forwarder {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.avail) == 0 {
		if p.config.Strategy == "pr" || p.config.Strategy == "prs" {
			return nil
		}
		return p.fwdrs[p.index.Add(1)%uint32(len(p.fwdrs))]
	}

	return p.next(dstAddr)
}

func (p *FwdrGroup) excludedByWeight() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, f := range p.fwdrs {
		if f.Weight() > 0 {
			return false
		}
	}
	return true
}

// Forwarders returns the forwarders in this group.
func (p *FwdrGroup) Forwarders() []*Forwarder {
	return p.fwdrs
}

// Priority returns the active priority of dialer.
func (p *FwdrGroup) Priority() uint32 { return p.priority.Load() }

// SetPriority sets the active priority of daler.
func (p *FwdrGroup) SetPriority(pri uint32) { p.priority.Store(pri) }

// init rebuilds the available forwarders under the group lock or during setup.
func (p *FwdrGroup) init() {
	weighted := p.config.Strategy == "pr" || p.config.Strategy == "prs"
	p.avail = nil
	p.SetPriority(0)
	for _, f := range p.fwdrs {
		if !f.Enabled() || (weighted && f.Weight() == 0) {
			continue
		}
		if len(p.avail) == 0 {
			p.SetPriority(f.Priority())
		}
		if f.Priority() < p.Priority() {
			break
		}
		p.avail = append(p.avail, f)
	}
}

// onStatusChanged will be called when fwdr's status changed.
func (p *FwdrGroup) onStatusChanged(fwdr *Forwarder) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	log.F("[group] %s: %s enabled=%t (%d of %d available)", p.name, fwdr.Addr(), fwdr.Enabled(), len(p.avail), len(p.fwdrs))
}

// Check runs the forwarder checks.
func (p *FwdrGroup) Check() {
	// check=disable is an activation path, so handle it before the single-forwarder shortcut.
	if strings.HasPrefix(p.config.Check, "disable") {
		for _, f := range p.fwdrs {
			f.Enable()
		}
		return
	}

	if len(p.fwdrs) == 1 {
		log.F("[group] %s: only 1 forwarder found, disable health checking", p.name)
		p.fwdrs[0].Enable()
		return
	}

	check := p.config.Check
	if !strings.Contains(check, "://") {
		check += "://"
	}

	u, err := url.Parse(check)
	if err != nil {
		log.F("[group] %s: parse check config error: %s, disable health checking", p.name, err)
		return
	}

	addr := u.Host
	timeout := time.Duration(p.config.CheckTimeout) * time.Second

	var checker Checker
	switch u.Scheme {
	case "tcp":
		checker = newTcpChecker(addr, timeout)
	case "http", "https":
		expect := "HTTP" // default: check the first 4 chars in response
		params, _ := url.ParseQuery(u.Fragment)
		if ex := params.Get("expect"); ex != "" {
			expect = ex
		}
		checker = newHttpChecker(addr, u.RequestURI(), expect, timeout, u.Scheme == "https")
	case "file":
		checker = newFileChecker(u.Host + u.Path)
	default:
		log.F("[group] %s: unknown scheme in check config `%s`, disable health checking", p.name, p.config.Check)
		return
	}

	log.F("[group] %s: using check config: %s", p.name, p.config.Check)

	for i := range p.fwdrs {
		go p.check(p.fwdrs[i], checker)
	}
}

func (p *FwdrGroup) check(fwdr *Forwarder, checker Checker) {
	wait := uint8(0)
	intval := time.Duration(p.config.CheckInterval) * time.Second

	for {
		time.Sleep(intval * time.Duration(wait))

		// check all forwarders at least one time
		if wait > 0 && (fwdr.Priority() < p.Priority()) {
			continue
		}

		if fwdr.Enabled() && p.config.CheckDisabledOnly {
			continue
		}

		elapsed, err := checker.Check(fwdr)
		if err != nil {
			if errors.Is(err, proxy.ErrNotSupported) {
				fwdr.SetMaxFailures(0)
				log.F("[check] %s: %s(%d), %s, stop checking", p.name, fwdr.Addr(), fwdr.Priority(), err)
				fwdr.Enable()
				break
			}

			wait++
			if wait > 16 {
				wait = 16
			}

			log.F("[check] %s: %s(%d), FAILED. error: %s", p.name, fwdr.Addr(), fwdr.Priority(), err)
			fwdr.Disable()
			continue
		}

		wait = 1
		p.setLatency(fwdr, elapsed)
		log.F("[check] %s: %s(%d), SUCCESS. Elapsed: %dms, Latency: %dms.",
			p.name, fwdr.Addr(), fwdr.Priority(), elapsed.Milliseconds(), time.Duration(fwdr.Latency()).Milliseconds())
		fwdr.Enable()
	}
}

func (p *FwdrGroup) setLatency(fwdr *Forwarder, elapsed time.Duration) {
	newLatency := int64(elapsed)
	if cnt := p.config.CheckLatencySamples; cnt > 1 {
		if lastLatency := fwdr.Latency(); lastLatency > 0 {
			newLatency = (lastLatency*(int64(cnt)-1) + int64(elapsed)) / int64(cnt)
		}
	}
	fwdr.SetLatency(newLatency)
}

// Round Robin.
func (p *FwdrGroup) scheduleRR(dstAddr string) *Forwarder {
	return p.avail[p.index.Add(1)%uint32(len(p.avail))]
}

// High Availability.
func (p *FwdrGroup) scheduleHA(dstAddr string) *Forwarder {
	return p.avail[0]
}

// Latency based High Availability.
func (p *FwdrGroup) scheduleLHA(dstAddr string) *Forwarder {
	oldfwdr, newfwdr := p.avail[0], p.avail[0]
	lowest := oldfwdr.Latency()
	for _, f := range p.avail {
		if f.Latency() < lowest {
			lowest = f.Latency()
			newfwdr = f
		}
	}
	tolerance := int64(p.config.CheckTolerance) * int64(time.Millisecond)
	if newfwdr.Latency() < (oldfwdr.Latency() - tolerance) {
		return newfwdr
	}
	return oldfwdr
}

// Probability Random.
// Weights are relative: each forwarder's probability = weight[i] / sum(weights).
// Zero-weight forwarders are excluded; an all-zero group has no candidate.
func (p *FwdrGroup) schedulePR(dstAddr string) *Forwarder {
	return weightedForwarder(p.avail)
}

func weightedForwarder(fwdrs []*Forwarder) *Forwarder {
	total := int64(0)
	for _, f := range fwdrs {
		total += f.Weight()
	}
	if total == 0 {
		return nil
	}
	r := rand.Int64N(total)
	for _, f := range fwdrs {
		w := f.Weight()
		if r < w {
			return f
		}
		r -= w
	}
	return fwdrs[0]
}

// Destination Hashing.
func (p *FwdrGroup) scheduleDH(dstAddr string) *Forwarder {
	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	return p.avail[fnv1a.Sum32()%uint32(len(p.avail))]
}
