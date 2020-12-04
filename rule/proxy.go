package rule

import (
	"github.com/nadoo/glider/rule/internal/matcher"
	"net"
	"strings"
	"sync"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// Proxy implements the proxy.Proxy interface with rule support.
type Proxy struct {
	main       *FwdrGroup
	all        []*FwdrGroup
	name2Group map[string]*FwdrGroup
	domainMap  sync.Map
	ipMap      sync.Map
	cidrMap    sync.Map
	routingA   *RoutingA
}

// NewProxy returns a new rule proxy.
func NewProxy(mainForwarders []string, mainStrategy *Strategy, rules []*Config) *Proxy {
	rd := &Proxy{
		main:       NewFwdrGroup("main", mainForwarders, mainStrategy),
		name2Group: make(map[string]*FwdrGroup),
		routingA:   mainStrategy.RoutingA,
	}

	rd.name2Group[OutProxy] = rd.main
	rd.name2Group[OutDirect] = NewFwdrGroup("", nil, mainStrategy)
	rd.name2Group[OutReject] = NewFwdrGroup("", []string{"reject://"}, mainStrategy)

	for _, r := range rules {
		group := NewFwdrGroup(r.Name, r.Forward, &r.Strategy)
		rd.all = append(rd.all, group)
		rd.name2Group[r.Name] = group

		for _, domain := range r.Domain {
			rd.domainMap.Store(strings.ToLower(domain), group)
		}

		for _, ip := range r.IP {
			rd.ipMap.Store(ip, group)
		}

		for _, s := range r.CIDR {
			if _, cidr, err := net.ParseCIDR(s); err == nil {
				rd.cidrMap.Store(cidr, group)
			}
		}
	}
	if rd.routingA != nil {
		rd.name2Group[OutDefault] = rd.name2Group[rd.routingA.DefaultOut]
	} else {
		rd.name2Group[OutDefault] = rd.main
	}

	// if there's any forwarder defined in main config, make sure they will be accessed directly.
	if len(mainForwarders) > 0 {
		for _, f := range rd.main.fwdrs {
			host, _, _ := net.SplitHostPort(f.addr)
			if ip := net.ParseIP(host); ip == nil {
				rd.domainMap.Store(strings.ToLower(host), rd.name2Group[OutDirect])
			}
		}
	}

	return rd
}

// Dial dials to targer addr and return a conn.
func (p *Proxy) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	return p.findDialer(network, addr).Dial(network, addr)
}

// DialUDP connects to the given address via the proxy.
func (p *Proxy) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, writeTo net.Addr, err error) {
	return p.findDialer(network, addr).DialUDP(network, addr)
}

// findDialer returns a dialer by dstAddr according to rule.
func (p *Proxy) findDialer(network string, dstAddr string) *FwdrGroup {
	host, port, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return p.main
	}

	// find ip
	var ip net.IP
	if ip = net.ParseIP(host); ip != nil {
		// check ip
		if proxy, ok := p.ipMap.Load(ip.String()); ok {
			return proxy.(*FwdrGroup)
		}

		var ret *FwdrGroup
		// check cidr
		p.cidrMap.Range(func(key, value interface{}) bool {
			cidr := key.(*net.IPNet)
			if cidr.Contains(ip) {
				ret = value.(*FwdrGroup)
				return false
			}
			return true
		})

		if ret != nil {
			return ret
		}
	}

	host = strings.ToLower(host)
	for i := len(host); i != -1; {
		i = strings.LastIndexByte(host[:i], '.')
		if proxy, ok := p.domainMap.Load(host[i+1:]); ok {
			return proxy.(*FwdrGroup)
		}
	}

	// check routingA
	if p.routingA != nil {
		for _, r := range p.routingA.Rules {
			matched := false
			for _, m := range r.Matchers {
				switch m.RuleType {
				case "domain":
					matched = m.Matcher.Match(host)
				case "tip":
					if ip != nil {
						matched = m.Matcher.Match(ip.String())
					}
				case "tport":
					matched = m.Matcher.Match(port)
				case "network":
					if network == "tcp" {
						matched = m.Matcher.Match(matcher.TCP)
					} else if network == "udp" {
						matched = m.Matcher.Match(matcher.UDP)
					}
				case "sport", "sip":
					// TODO: UNSUPPORTED
				case "app":
					// TODO: UNSUPPORTED
				}
				if matched {
					break
				}
			}
			if matched {
				if g, ok := p.name2Group[r.Out]; ok {
					return g
				} else {
					log.F("invalid out rule: ", r.Out)
				}
			}
		}
		return p.name2Group[OutDefault]
	}

	return p.main
}

// NextDialer returns next dialer according to rule.
func (p *Proxy) NextDialer(network, dstAddr string) proxy.Dialer {
	return p.findDialer(network, dstAddr).NextDialer(dstAddr)
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
func (p *Proxy) AddDomainIP(domain, ip string) error {
	if ip != "" {
		domain = strings.ToLower(domain)
		for i := len(domain); i != -1; {
			i = strings.LastIndexByte(domain[:i], '.')
			if dialer, ok := p.domainMap.Load(domain[i+1:]); ok {
				p.ipMap.Store(ip, dialer)
				log.F("[rule] add ip=%s, based on rule: domain=%s & domain/ip: %s/%s\n",
					ip, domain[i+1:], domain, ip)
			}
		}
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
