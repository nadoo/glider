package rule

import (
	"net"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

type Rule struct {
	name       string
	forwarders *FwdrGroup
	domains    []string
	ips        []netip.Addr
	cidrs      []netip.Prefix
}

// Proxy implements the proxy.Proxy interface with rule support.
type Proxy struct {
	main  *FwdrGroup
	rules []*Rule
}

// NewProxy returns a new rule proxy.
func NewProxy(mainForwarders []string, mainStrategy *Strategy, rules []*Config) *Proxy {
	proxy := &Proxy{main: NewFwdrGroup("main", mainForwarders, mainStrategy)}

	for _, r := range rules {
		name := strings.TrimSuffix(filepath.Base(r.RulePath), filepath.Ext(r.RulePath))
		forwarders := NewFwdrGroup(name, r.Forward, &r.Strategy)
		rule := &Rule{name: name, forwarders: forwarders}

		for _, domain := range r.Domain {
			rule.domains = append(rule.domains, strings.ToLower(domain))
			log.F("[rule] %s: has domain rule for %s", name, domain)
		}

		for _, s := range r.IP {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				log.F("[rule] %s: parse ip error: %s", name, err)
				continue
			}
			rule.ips = append(rule.ips, ip)
			log.F("[rule] %s: has IP rule for %s", name, ip.String())
		}

		for _, s := range r.CIDR {
			cidr, err := netip.ParsePrefix(s)
			if err != nil {
				log.F("[rule] parse cidr error: %s", err)
				continue
			}
			rule.cidrs = append(rule.cidrs, cidr)
			log.F("[rule] %s: has CIDR rule for %s", name, cidr.String())
		}

		proxy.rules = append(proxy.rules, rule)
	}

	direct := NewFwdrGroup("direct", nil, mainStrategy)
	directRule := &Rule{name: "direct", forwarders: direct, domains: []string{"direct"}}

	// if there's any forwarder defined in main config, make sure they will be accessed directly.
	if len(mainForwarders) > 0 {
		for _, f := range proxy.main.fwdrs {
			addr := strings.Split(f.addr, ",")[0]
			host, _, _ := net.SplitHostPort(addr)
			if _, err := netip.ParseAddr(host); err != nil {
				directRule.domains = append(directRule.domains, strings.ToLower(host))
				log.F("[rule] direct: has domain rule for %s", host)
			}
		}
	}

	proxy.rules = append(proxy.rules, directRule)

	return proxy
}

func (r *Rule) checkDomain(host string) bool {
	host = strings.ToLower(host)
	for i := len(host); i != -1; {
		i = strings.LastIndexByte(host[:i], '.')
		for _, domain := range r.domains {
			if domain == host[i+1:] {
				return true
			}
		}
	}

	return false
}

func (r *Rule) checkIP(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		// check ip
		for _, addr := range r.ips {
			if addr == ip {
				return true
			}
		}

		// check cidr
		for _, prefix := range r.cidrs {
			if prefix.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// checkMatch checks whether the given dstAddr matches the rules
func (r *Rule) checkMatch(dstAddr string) bool {
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return false
	}

	return r.checkIP(host) || r.checkDomain(host)
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
	for _, rule := range p.rules {
		if rule.checkMatch(dstAddr) {
			return rule.forwarders
		}
	}

	return p.main
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
	for _, rule := range p.rules {
		if rule.checkDomain(domain) {
			rule.ips = append(rule.ips, ip)
		}
	}

	return nil
}

// Check checks availability of forwarders inside proxy.
func (p *Proxy) Check() {
	p.main.Check()

	for _, rule := range p.rules {
		rule.forwarders.Check()
	}
}
