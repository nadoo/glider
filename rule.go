package main

import (
	"log"
	"net"
	"strings"
	"sync"
)

// RuleDialer struct
type RuleDialer struct {
	gDialer Dialer

	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewRuleDialer returns a new rule dialer
func NewRuleDialer(rules []*RuleConf, gDialer Dialer) *RuleDialer {
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

		sDialer := NewStrategyDialer(r.Strategy, forwarders, r.CheckWebSite, r.CheckDuration)

		for _, domain := range r.Domain {
			rd.domainMap.Store(domain, sDialer)
		}

		for _, ip := range r.IP {
			rd.ipMap.Store(ip, sDialer)
		}

		for _, s := range r.CIDR {
			if _, cidr, err := net.ParseCIDR(s); err == nil {
				rd.cidrMap.Store(cidr, sDialer)
			}
		}

	}

	return rd
}

// Addr returns RuleDialer's address, always be "RULES"
func (rd *RuleDialer) Addr() string { return "RULES" }

// NextDialer return next dialer according to rule
func (rd *RuleDialer) NextDialer(dstAddr string) Dialer {

	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		// logf("proxy-rule SplitHostPort ERROR: %s", err)
		return rd.gDialer
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check ip
		if dialer, ok := rd.ipMap.Load(ip.String()); ok {
			return dialer.(Dialer)
		}

		var ret Dialer
		// check cidr
		rd.cidrMap.Range(func(key, value interface{}) bool {
			cidr := key.(*net.IPNet)
			if cidr.Contains(ip) {
				ret = value.(Dialer)
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
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		// find in domainMap
		if dialer, ok := rd.domainMap.Load(domain); ok {
			return dialer.(Dialer)
		}
	}

	return rd.gDialer
}

// Dial dials to targer addr and return a conn
func (rd *RuleDialer) Dial(network, addr string) (net.Conn, error) {
	return rd.NextDialer(addr).Dial(network, addr)
}

// AddDomainIP used to update ipMap rules according to domainMap rule
func (rd *RuleDialer) AddDomainIP(domain, ip string) error {
	if ip != "" {
		domainParts := strings.Split(domain, ".")
		length := len(domainParts)
		for i := length - 2; i >= 0; i-- {
			pDomain := strings.Join(domainParts[i:length], ".")

			// find in domainMap
			if dialer, ok := rd.domainMap.Load(pDomain); ok {
				rd.ipMap.Store(ip, dialer)
				logf("rule add `ip=%s`, based on rule: `domain=%s`, domain/ip: %s/%s\n", ip, pDomain, domain, ip)
			}
		}

	}
	return nil
}
