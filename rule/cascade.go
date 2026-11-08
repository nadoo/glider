package rule

import (
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nadoo/glider/proxy"
)

const defaultCascadeAttempts = 1

// cascadeDialer wraps a chain dialer with the selected forwarders for
// address reporting and enable-on-success.
type cascadeDialer struct {
	proxy.Dialer
	selected []*Forwarder
}

func (cd *cascadeDialer) Addr() string {
	var addrs []string
	for _, f := range cd.selected {
		addrs = append(addrs, f.Addr())
	}
	if len(addrs) == 0 {
		return cd.Dialer.Addr()
	}
	return "cascade: " + strings.Join(addrs, " -> ")
}

// CascadeGroup groups forwarders by cascade level and chains them at dial time.
type CascadeGroup struct {
	levels       []*FwdrGroup
	intface      string
	dialTimeout  time.Duration
	relayTimeout time.Duration
	strict       bool
	overrides    []RetryOptions
}

// NewCascadeGroup returns a CascadeGroup from forwarders grouped by cascade level.
func NewCascadeGroup(fwdrs []*Forwarder, c *Strategy) *CascadeGroup {
	groups := make(map[uint32][]*Forwarder)
	for _, f := range fwdrs {
		groups[f.Cascade()] = append(groups[f.Cascade()], f)
	}
	var keys []uint32
	for k := range groups {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	cg := &CascadeGroup{
		intface:      c.IntFace,
		dialTimeout:  time.Duration(c.DialTimeout) * time.Second,
		relayTimeout: time.Duration(c.RelayTimeout) * time.Second,
		strict:       c.Strategy == "prs",
	}
	for _, k := range keys {
		// Per-field precedence: specific cascade > global (level 0) > forwarder.
		// Fill only unset fields from the global entry; forwarder defaults are
		// applied later in cascadeSelection.next.
		options := c.Cascade[k]
		global := c.Cascade[0]
		if options.Attempts == nil {
			options.Attempts = global.Attempts
		}
		if options.Retries == nil {
			options.Retries = global.Retries
		}
		if options.RetryDelay == nil {
			options.RetryDelay = global.RetryDelay
		}
		if options.AttemptDelay == nil {
			options.AttemptDelay = global.AttemptDelay
		}
		cg.overrides = append(cg.overrides, options)
		cg.levels = append(cg.levels, newFwdrGroup("cascade-"+strconv.FormatUint(uint64(k), 10), groups[k], c))
	}
	return cg
}

func (cg *CascadeGroup) chain(states []cascadeSelection, previous *cascadeDialer) (*cascadeDialer, time.Duration, error) {
	var selected []*Forwarder
	var delay time.Duration
	for i, group := range cg.levels {
		if group.excludedByWeight() {
			continue
		}
		f, pause, err := states[i].next(group, cg.overrides[i])
		if err != nil {
			return nil, 0, err
		}
		if f == nil {
			if cg.strict {
				return nil, 0, errNoAvailableForwarder
			}
			continue
		}
		selected = append(selected, f)
		delay = max(delay, pause)
	}
	if len(selected) == 0 {
		if cg.strict {
			return nil, 0, errNoAvailableForwarder
		}
		d, err := proxy.NewDirect(cg.intface, cg.dialTimeout, cg.relayTimeout)
		return &cascadeDialer{Dialer: d}, 0, err
	}
	if previous != nil && slices.Equal(previous.selected, selected) {
		// The redraw picked the same forwarders. Reuse the built chain instead
		// of stacking the dialers again; the draw was still consumed above.
		return previous, delay, nil
	}
	base := selected[0].Dialer
	for _, f := range selected[1:] {
		var err error
		base, _, err = chainFromURL(f.URL(), base)
		if err != nil {
			return nil, 0, err
		}
	}
	return &cascadeDialer{Dialer: base, selected: selected}, delay, nil
}

// connect dials complete chains until success or budget exhaustion. states
// are request-local: each level advances its retry/draw budget independently.
// After every level's budget is spent, pr falls back to one direct dial while
// prs reports the last error without any fallback.
func (cg *CascadeGroup) connect(dial func(proxy.Dialer) error) (proxy.Dialer, error) {
	states := make([]cascadeSelection, len(cg.levels))
	var previous *cascadeDialer
	var lastErr error
	for {
		chain, delay, err := cg.chain(states, previous)
		if errors.Is(err, errCascadeBudget) {
			break
		}
		if err != nil {
			return &errorDialer{err}, err
		}
		time.Sleep(delay)
		lastErr = dial(chain)
		if lastErr == nil {
			for _, f := range chain.selected {
				f.Enable()
			}
			return chain, nil
		}
		if len(chain.selected) == 0 {
			return chain, lastErr
		}
		previous = chain
	}
	if !cg.strict {
		d, err := proxy.NewDirect(cg.intface, cg.dialTimeout, cg.relayTimeout)
		if err != nil {
			return &errorDialer{err}, err
		}
		return d, dial(d)
	}
	if lastErr == nil {
		lastErr = errCascadeBudget
	}
	return &errorDialer{lastErr}, lastErr
}

func (cg *CascadeGroup) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	var conn net.Conn
	d, err := cg.connect(func(d proxy.Dialer) error {
		if network == "udp" && d.Addr() == "DIRECT" {
			return proxy.ErrNotSupported
		}
		var err error
		conn, err = d.Dial(network, addr)
		if err != nil && conn != nil {
			conn.Close()
			conn = nil
		}
		return err
	})
	return conn, d, err
}

// DialUDP connects to addr through the cascade chain.
func (cg *CascadeGroup) DialUDP(network, addr string) (net.PacketConn, proxy.UDPDialer, error) {
	var conn net.PacketConn
	d, err := cg.connect(func(d proxy.Dialer) error {
		var err error
		conn, err = d.DialUDP(network, addr)
		if err != nil && conn != nil {
			conn.Close()
			conn = nil
		}
		return err
	})
	return conn, d, err
}

// Forwarders returns all forwarders across all cascade levels.
func (cg *CascadeGroup) Forwarders() []*Forwarder {
	var all []*Forwarder
	for _, group := range cg.levels {
		all = append(all, group.Forwarders()...)
	}
	return all
}

func (cg *CascadeGroup) NextDialer(dstAddr string) proxy.Dialer {
	if !cg.strict {
		available := false
		for _, group := range cg.levels {
			group.mu.RLock()
			available = available || len(group.avail) > 0
			group.mu.RUnlock()
		}
		if !available {
			d, err := proxy.NewDirect(cg.intface, cg.dialTimeout, cg.relayTimeout)
			if err != nil {
				return &errorDialer{err}
			}
			return d
		}
	}
	return &cascadeAdapter{group: cg}
}

type cascadeAdapter struct{ group *CascadeGroup }

func (d *cascadeAdapter) Addr() string { return "cascade" }
func (d *cascadeAdapter) Dial(network, addr string) (net.Conn, error) {
	c, _, err := d.group.Dial(network, addr)
	return c, err
}
func (d *cascadeAdapter) DialUDP(network, addr string) (net.PacketConn, error) {
	c, _, err := d.group.DialUDP(network, addr)
	return c, err
}

// Check runs health checks on all cascade levels.
func (cg *CascadeGroup) Check() {
	for _, group := range cg.levels {
		group.Check()
	}
}

var errCascadeBudget = errors.New("cascade selection budget exhausted")

type cascadeSelection struct {
	forwarder *Forwarder
	retries   int
	draws     int
}

func (s *cascadeSelection) next(group *FwdrGroup, override RetryOptions) (*Forwarder, time.Duration, error) {
	group.mu.RLock()
	defer group.mu.RUnlock()
	if len(group.avail) == 0 {
		s.forwarder = nil
		return nil, 0, nil
	}
	if s.forwarder != nil && s.retries > 0 && slices.Contains(group.avail, s.forwarder) {
		// Retry the same forwarder, but only while it is still available: a
		// health check or maxfailures may have disabled it between attempts.
		s.retries--
		delay := s.forwarder.retryDelay
		if override.RetryDelay != nil {
			delay = time.Duration(*override.RetryDelay) * time.Millisecond
		}
		return s.forwarder, delay, nil
	}
	attempts := defaultCascadeAttempts
	if override.Attempts != nil {
		attempts = *override.Attempts
	}
	if s.draws >= attempts {
		return nil, 0, errCascadeBudget
	}
	f := weightedForwarder(group.avail)
	if f == nil {
		return nil, 0, errNoAvailableForwarder
	}
	var delay time.Duration
	if s.draws > 0 && override.AttemptDelay != nil {
		delay = time.Duration(*override.AttemptDelay) * time.Millisecond
	}
	s.draws++
	s.forwarder = f
	s.retries = f.retries
	if override.Retries != nil {
		s.retries = *override.Retries
	}
	return f, delay, nil
}
