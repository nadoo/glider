package main

import (
	"log"
	"net"
	"strings"
)

// RuleDialer .
type RuleDialer struct {
	gDialer Dialer

	domainMap map[string]Dialer
	ipMap     map[string]Dialer
	cidrMap   map[string]Dialer
}

// NewRuleDialer .
func NewRuleDialer(rules []*RuleConf, gDialer Dialer) Dialer {

	if len(rules) == 0 {
		return gDialer
	}

	rd := &RuleDialer{gDialer: gDialer}

	for _, r := range rules {
		var forwarders []Dialer
		for _, chain := range r.Forward {
			var forward Dialer
			var err error
			for _, url := range strings.Split(chain, ",") {
				forward, err = DialerFromURL(url, forward)
				if err != nil {
					log.Fatal(err)
				}
			}
			forwarders = append(forwarders, forward)
		}

		sd := NewStrategyDialer(r.Strategy, forwarders, r.CheckWebSite, r.CheckDuration)

		rd.domainMap = make(map[string]Dialer)
		for _, domain := range r.Domain {
			rd.domainMap[domain] = sd
		}

		rd.ipMap = make(map[string]Dialer)
		for _, ip := range r.IP {
			rd.ipMap[ip] = sd
		}

		// dnsserver should use rule forwarder too
		for _, dnss := range r.DNSServer {
			ip, _, err := net.SplitHostPort(dnss)
			if err != nil {
				logf("SplitHostPort ERROR: %s", err)
				continue
			}
			rd.ipMap[ip] = sd
		}

		rd.cidrMap = make(map[string]Dialer)
		for _, cidr := range r.CIDR {
			rd.cidrMap[cidr] = sd
		}
	}

	return rd
}

func (rd *RuleDialer) Addr() string { return "RULES" }

func (p *RuleDialer) NextDialer(dstAddr string) Dialer {

	// TODO: change to index finders
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		logf("SplitHostPort ERROR: %s", err)
		return p.gDialer
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check ip
		if d, ok := p.ipMap[ip.String()]; ok {
			return d
		}

		// check cidr
		// TODO: do not parse cidr every time
		for cidrStr, d := range p.cidrMap {
			if _, net, err := net.ParseCIDR(cidrStr); err == nil {
				if net.Contains(ip) {
					return d
				}
			}
		}
	}

	domainParts := strings.Split(host, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		// find in domainMap
		if d, ok := p.domainMap[domain]; ok {
			return d
		}
	}

	return p.gDialer
}

func (rd *RuleDialer) Dial(network, addr string) (net.Conn, error) {
	d := rd.NextDialer(addr)
	return d.Dial(network, addr)
}
