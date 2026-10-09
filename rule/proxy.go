package rule

import (
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
	all       []*FwdrGroup
	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewProxy returns a new rule proxy.
func NewProxy(mainForwarders []string, mainStrategy *Strategy, rules []*Config) *Proxy {
	rd := &Proxy{main: NewFwdrGroup("main", mainForwarders, mainStrategy)}

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
	if len(mainForwarders) > 0 {
		for _, f := range rd.main.fwdrs {
			addr, _, _ := strings.Cut(f.addr, ",")
			host, _, _ := net.SplitHostPort(addr)
			if _, err := netip.ParseAddr(host); err != nil {
				rd.domainMap.Store(normalizeDomain(host), direct)
			}
		}
	}

	return rd
}

// Dial dials to targer addr and return a conn.
func (p *Proxy) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	return p.findDialer(addr).Dial(network, addr)
}

// DialUDP connects to the given address via the proxy.
func (p *Proxy) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	return p.findDialer(addr).DialUDP(network, addr)
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
func (p *Proxy) NextDialer(dstAddr string) proxy.Dialer {
	return p.findDialer(dstAddr).NextDialer(dstAddr)
}

// Record records result while using the dialer from proxy.
func (p *Proxy) Record(dialer proxy.Dialer, success bool) {
	if fwdr, ok := dialer.(*Forwarder); ok {
		if !success {
			fwdr.IncFailures()
			return
		}
		fwdr.Enable()
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
	p.main.Check()

	for _, fwdrGroup := range p.all {
		fwdrGroup.Check()
	}
}
