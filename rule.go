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

		sd := NewStrategyDialer(r.Strategy, forwarders, r.CheckWebSite, r.CheckDuration)

		for _, domain := range r.Domain {
			rd.domainMap.Store(domain, sd)
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

// Addr returns RuleDialer's address, always be "RULES"
func (rd *RuleDialer) Addr() string { return "RULES" }

// NextDialer return next dialer according to rule
func (rd *RuleDialer) NextDialer(dstAddr string) Dialer {

	// TODO: change to index finders
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		// TODO: check here
		// logf("proxy-rule SplitHostPort ERROR: %s", err)
		return rd.gDialer
	}

	// find ip
	if ip := net.ParseIP(host); ip != nil {
		// check ip
		if d, ok := rd.ipMap.Load(ip.String()); ok {
			return d.(Dialer)
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
		if d, ok := rd.domainMap.Load(domain); ok {
			return d.(Dialer)
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
			domain := strings.Join(domainParts[i:length], ".")

			// find in domainMap
			if d, ok := rd.domainMap.Load(domain); ok {
				rd.ipMap.Store(ip, d)
				logf("rule: add domain: %s, ip: %s\n", domain, ip)
			}
		}

	}
	return nil
}
