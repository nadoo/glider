package main

import (
	"net"
	"strings"
)

// RulesForwarder .
type RulesForwarder struct {
	globalForwarder Proxy

	domainMap map[string]Proxy
	ipMap     map[string]Proxy
	cidrMap   map[string]Proxy
}

// NewRulesForwarder .
func NewRulesForwarder(ruleForwarders []*RuleForwarder, globalForwarder Proxy) Proxy {

	if len(ruleForwarders) == 0 {
		return globalForwarder
	}

	p := &RulesForwarder{globalForwarder: globalForwarder}

	for _, f := range ruleForwarders {
		p.domainMap = make(map[string]Proxy)
		for _, domain := range f.Domain {
			p.domainMap[domain] = f.Proxy
		}

		p.ipMap = make(map[string]Proxy)
		for _, ip := range f.IP {
			p.ipMap[ip] = f.Proxy
		}

		p.cidrMap = make(map[string]Proxy)
		for _, cidr := range f.CIDR {
			p.cidrMap[cidr] = f.Proxy
		}
	}

	return p
}

func (p *RulesForwarder) Addr() string        { return "rules forwarder" }
func (p *RulesForwarder) ListenAndServe()     {}
func (p *RulesForwarder) Serve(c net.Conn)    {}
func (p *RulesForwarder) CurrentProxy() Proxy { return p.globalForwarder.CurrentProxy() }

func (p *RulesForwarder) GetProxy(dstAddr string) Proxy {

	// TODO: change to index finders
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		logf("SplitHostPort ERROR: %s", err)
		return p.globalForwarder.GetProxy(dstAddr)
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check ip
		if proxy, ok := p.ipMap[ip.String()]; ok {
			return proxy
		}

		// check cidr
		// TODO: do not parse cidr every time
		for cidrStr, proxy := range p.cidrMap {
			if _, net, err := net.ParseCIDR(cidrStr); err == nil {
				if net.Contains(ip) {
					return proxy
				}
			}
		}
	}

	domainParts := strings.Split(host, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		// find in domainMap
		if proxy, ok := p.domainMap[domain]; ok {
			return proxy
		}
	}

	return p.globalForwarder.GetProxy(dstAddr)
}

func (p *RulesForwarder) NextProxy() Proxy {
	return p.globalForwarder.NextProxy()
}

func (p *RulesForwarder) Enabled() bool         { return true }
func (p *RulesForwarder) SetEnable(enable bool) {}

func (p *RulesForwarder) Dial(network, addr string) (net.Conn, error) {
	return p.GetProxy(addr).Dial(network, addr)
}
