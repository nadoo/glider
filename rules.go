package main

import (
	"net"
	"strings"
)

type rulesForwarder struct {
	globalForwarder Proxy

	domainMap map[string]Proxy
	ipMap     map[string]Proxy
	cidrMap   map[string]Proxy
}

// newRulesForwarder .
func newRulesForwarder(ruleForwarders []*ruleForwarder, globalForwarder Proxy) Proxy {

	if len(ruleForwarders) == 0 {
		return globalForwarder
	}

	p := &rulesForwarder{globalForwarder: globalForwarder}

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

func (p *rulesForwarder) Addr() string        { return "rule forwarder" }
func (p *rulesForwarder) ListenAndServe()     {}
func (p *rulesForwarder) Serve(c net.Conn)    {}
func (p *rulesForwarder) CurrentProxy() Proxy { return p.globalForwarder.CurrentProxy() }

func (p *rulesForwarder) GetProxy(dstAddr string) Proxy {

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

func (p *rulesForwarder) NextProxy() Proxy {
	return p.globalForwarder.NextProxy()
}

func (p *rulesForwarder) Enabled() bool         { return true }
func (p *rulesForwarder) SetEnable(enable bool) {}

func (p *rulesForwarder) Dial(network, addr string) (net.Conn, error) {
	return p.GetProxy(addr).Dial(network, addr)
}
