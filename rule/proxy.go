package rule

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Proxy implements the proxy.Proxy interface with rule support.
type Proxy struct {
	main      *FwdrGroup
	cascade   *CascadeGroup
	all       []*FwdrGroup
	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewProxy returns a new rule proxy.
func NewProxy(mainForwarders []string, mainStrategy *Strategy, rules []*Config) *Proxy {
	rd := &Proxy{}

	cascadeFwdrs, hasCascade, err := parseMainForwarders(mainForwarders, mainStrategy)
	if err != nil {
		log.Fatal(err)
	}

	if hasCascade {
		rd.cascade = NewCascadeGroup(cascadeFwdrs, mainStrategy)
		rd.main = NewFwdrGroup("main", nil, mainStrategy)
	} else if len(cascadeFwdrs) > 0 {
		rd.main = newFwdrGroup("main", cascadeFwdrs, mainStrategy)
	} else {
		rd.main = NewFwdrGroup("main", nil, mainStrategy)
	}

	for _, r := range rules {
		group := NewFwdrGroup(r.RulePath, r.Forward, &r.Strategy)
		rd.all = append(rd.all, group)

		for _, domain := range r.Domain {
			rd.domainMap.Store(normalizeDomain(domain), group)
		}

		for _, s := range r.IP {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				log.F("[rule] parse ip error: %s", err)
				continue
			}
			rd.ipMap.Store(ip, group)
		}

		for _, s := range r.CIDR {
			cidr, err := netip.ParsePrefix(s)
			if err != nil {
				log.F("[rule] parse cidr error: %s", err)
				continue
			}
			rd.cidrMap.Store(cidr, group)
		}
	}

	direct := NewFwdrGroup("", nil, mainStrategy)
	rd.domainMap.Store("direct", direct)

	// if there's any forwarder defined in main config, make sure they will be accessed directly.
	mainFwdrs := rd.getMainForwarders()
	if len(mainForwarders) > 0 {
		for _, f := range mainFwdrs {
			addr, _, _ := strings.Cut(f.addr, ",")
			host, _, _ := net.SplitHostPort(addr)
			if _, err := netip.ParseAddr(host); err != nil {
				rd.domainMap.Store(normalizeDomain(host), direct)
			}
		}
	}

	return rd
}

func parseMainForwarders(mainForwarders []string, c *Strategy) ([]*Forwarder, bool, error) {
	var fwdrs []*Forwarder
	var hasCascade bool

	for _, s := range mainForwarders {
		f, err := newForwarderFromConfig(s, c)
		if err != nil {
			return nil, false, err
		}
		fwdrs = append(fwdrs, f)
		if f.Cascade() > 0 {
			hasCascade = true
		}
	}
	for level := range c.Cascade {
		if level == 0 {
			if !hasCascade {
				return nil, false, fmt.Errorf("global cascade options require cascade forwarders")
			}
			continue
		}
		found := false
		for _, f := range fwdrs {
			found = found || f.Cascade() == level
		}
		if !found {
			return nil, false, fmt.Errorf("cascade level %d has no forwarders", level)
		}
	}
	if hasCascade {
		firstLevel := ^uint32(0)
		for _, f := range fwdrs {
			if f.Cascade() < firstLevel {
				firstLevel = f.Cascade()
			}
		}
		if c.Strategy != "pr" && c.Strategy != "prs" {
			return nil, false, fmt.Errorf("cascade requires strategy pr or prs")
		}
		for _, f := range fwdrs {
			if f.Cascade() == 0 {
				return nil, false, fmt.Errorf("forwarder %s must set cascade > 0 when cascade is used", f.Addr())
			}
			raw, _, _ := strings.Cut(f.URL(), "#")
			for i, layer := range strings.Split(raw, ",") {
				scheme, _, _ := strings.Cut(layer, ":")
				switch strings.ToLower(scheme) {
				case "ssh", "smux":
					if f.Cascade() != firstLevel {
						return nil, false, fmt.Errorf("%s requires the first cascade level to reuse its session", scheme)
					}
				case "kcp", "unix", "vsock":
					if f.Cascade() != firstLevel || i != 0 {
						return nil, false, fmt.Errorf("%s must be the first transport in the first cascade level", scheme)
					}
				}
			}
		}
	}

	return fwdrs, hasCascade, nil
}

// Dial dials to targer addr and return a conn.
func (p *Proxy) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	fg := p.findDialer(addr)
	if fg == p.main && p.cascade != nil {
		return p.cascade.Dial(network, addr)
	}
	return fg.Dial(network, addr)
}

// DialUDP connects to the given address via the proxy.
func (p *Proxy) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	fg := p.findDialer(addr)
	if fg == p.main && p.cascade != nil {
		return p.cascade.DialUDP(network, addr)
	}
	return fg.DialUDP(network, addr)
}

// findDialer returns a dialer by dstAddr according to rule.
func (p *Proxy) findDialer(dstAddr string) *FwdrGroup {
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return p.main
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		// check ip
		if proxy, ok := p.ipMap.Load(ip); ok {
			return proxy.(*FwdrGroup)
		}

		// check cidr
		var ret *FwdrGroup
		p.cidrMap.Range(func(key, value any) bool {
			if key.(netip.Prefix).Contains(ip) {
				ret = value.(*FwdrGroup)
				return false
			}
			return true
		})

		if ret != nil {
			return ret
		}
	}

	// Check the full host before its parent domains so the most specific
	// matching rule wins (for example, a.example.com before example.com).
	if group, ok := p.findDomainGroup(host); ok {
		return group
	}

	return p.main
}

func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(domain), ".")
}

// findDomainGroup finds the most specific group matching domain.
func (p *Proxy) findDomainGroup(domain string) (*FwdrGroup, bool) {
	domain = normalizeDomain(domain)
	for {
		if group, ok := p.domainMap.Load(domain); ok {
			return group.(*FwdrGroup), true
		}

		dot := strings.IndexByte(domain, '.')
		if dot == -1 {
			return nil, false
		}
		domain = domain[dot+1:]
	}
}

// NextDialer returns next dialer according to rule.
// It never returns nil so that callers can safely call Addr()/Dial() on it.
func (p *Proxy) NextDialer(dstAddr string) proxy.Dialer {
	fg := p.findDialer(dstAddr)

	var d proxy.Dialer
	if fg == p.main && p.cascade != nil {
		d = p.cascade.NextDialer(dstAddr)
	} else if fwdr := fg.NextDialer(dstAddr); fwdr != nil {
		d = fwdr
	}

	if d == nil {
		return &errorDialer{errNoAvailableForwarder}
	}
	return d
}

// Record records result while using the dialer from proxy.
func (p *Proxy) Record(dialer proxy.Dialer, success bool) {
	if fwdr, ok := dialer.(*Forwarder); ok {
		if !success {
			fwdr.IncFailures()
			return
		}
		fwdr.Enable()
		return
	}

	if cd, ok := dialer.(*cascadeDialer); ok {
		// Cascade hops are disabled only by health checks, not by relay errors,
		// so never increment failures here. Successful relays re-enable hops.
		if !success {
			return
		}
		for _, f := range cd.selected {
			f.Enable()
		}
		return
	}
}

// AddDomainIP used to update ipMap rules according to domainMap rule.
func (p *Proxy) AddDomainIP(domain string, ip netip.Addr) error {
	if group, ok := p.findDomainGroup(domain); ok {
		p.ipMap.Store(ip, group)
	}
	return nil
}

// Check checks availability of forwarders inside proxy.
func (p *Proxy) Check() {
	if p.cascade != nil {
		p.cascade.Check()
	} else {
		p.main.Check()
	}

	for _, fwdrGroup := range p.all {
		fwdrGroup.Check()
	}
}

func (p *Proxy) getMainForwarders() []*Forwarder {
	if p.cascade != nil {
		return p.cascade.Forwarders()
	}
	return p.main.fwdrs
}
