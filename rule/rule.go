package rule

import (
	"net"
	"strings"
	"sync"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/strategy"
)

// Proxy struct
type Proxy struct {
	proxy   *strategy.Proxy
	proxies []*strategy.Proxy

	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewProxy returns a new rule proxy
func NewProxy(rules []*Config, proxy *strategy.Proxy) *Proxy {
	rd := &Proxy{proxy: proxy}

	for _, r := range rules {
		sd := strategy.NewProxy(r.Forward, &r.StrategyConfig)
		rd.proxies = append(rd.proxies, sd)

		for _, domain := range r.Domain {
			rd.domainMap.Store(strings.ToLower(domain), sd)
		}

		for _, ip := range r.IP {
			rd.ipMap.Store(ip, sd)
		}

		for _, s := range r.CIDR {
			if _, cidr, err := net.ParseCIDR(s); err == nil {
				rd.cidrMap.Store(cidr, sd)
			}
		}
	}

	return rd
}

// Dial dials to targer addr and return a conn
func (p *Proxy) Dial(network, addr string) (net.Conn, string, error) {
	return p.nextProxy(addr).Dial(network, addr)
}

// DialUDP connects to the given address via the proxy
func (p *Proxy) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return p.nextProxy(addr).DialUDP(network, addr)
}

// nextProxy return next proxy according to rule
func (p *Proxy) nextProxy(dstAddr string) *strategy.Proxy {
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		// logf("[rule] SplitHostPort ERROR: %s", err)
		return p.proxy
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check ip
		if proxy, ok := p.ipMap.Load(ip.String()); ok {
			return proxy.(*strategy.Proxy)
		}

		var ret *strategy.Proxy
		// check cidr
		p.cidrMap.Range(func(key, value interface{}) bool {
			cidr := key.(*net.IPNet)
			if cidr.Contains(ip) {
				ret = value.(*strategy.Proxy)
				return false
			}

			return true
		})

		if ret != nil {
			return ret
		}

	}

	domainParts := strings.Split(host, ".")
	length := len(domainParts)
	for i := length - 1; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		// find in domainMap
		if proxy, ok := p.domainMap.Load(domain); ok {
			return proxy.(*strategy.Proxy)
		}
	}

	return p.proxy
}

// NextDialer return next dialer according to rule
func (p *Proxy) NextDialer(dstAddr string) proxy.Dialer {
	return p.nextProxy(dstAddr).NextDialer(dstAddr)
}

// AddDomainIP used to update ipMap rules according to domainMap rule
func (p *Proxy) AddDomainIP(domain, ip string) error {
	if ip != "" {
		domainParts := strings.Split(domain, ".")
		length := len(domainParts)
		for i := length - 1; i >= 0; i-- {
			pDomain := strings.ToLower(strings.Join(domainParts[i:length], "."))

			// find in domainMap
			if dialer, ok := p.domainMap.Load(pDomain); ok {
				p.ipMap.Store(ip, dialer)
				log.F("[rule] add ip=%s, based on rule: domain=%s & domain/ip: %s/%s\n", ip, pDomain, domain, ip)
			}
		}
	}
	return nil
}

// Check .
func (p *Proxy) Check() {
	p.proxy.Check()

	for _, d := range p.proxies {
		d.Check()
	}
}
