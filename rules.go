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
	p := &rulesForwarder{globalForwarder: globalForwarder}

	for _, f := range ruleForwarders {
		p.domainMap = make(map[string]Proxy)
		for _, domain := range f.Domain {
			p.domainMap[domain] = f.sForwarder
		}

		p.ipMap = make(map[string]Proxy)
		for _, ip := range f.IP {
			p.ipMap[ip] = f.sForwarder
		}

		p.cidrMap = make(map[string]Proxy)
		for _, cidr := range f.CIDR {
			p.cidrMap[cidr] = f.sForwarder
		}
	}

	return p
}

func (p *rulesForwarder) Addr() string        { return "rule forwarder" }
func (p *rulesForwarder) ListenAndServe()     {}
func (p *rulesForwarder) Serve(c net.Conn)    {}
func (p *rulesForwarder) CurrentProxy() Proxy { return p.globalForwarder.CurrentProxy() }

func (p *rulesForwarder) GetProxy(dstAddr string) Proxy {

	logf("dstAddr: %s", dstAddr)

	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		logf("%s", err)
		return p.globalForwarder.GetProxy(dstAddr)
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check cidr

		// check ip
		if p, ok := p.ipMap[ip.String()]; ok {
			return p
		}
	}

	domainParts := strings.Split(host, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		// find in domainMap
		if p, ok := p.domainMap[domain]; ok {
			return p
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
