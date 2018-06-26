package main

import (
	"net"
	"strings"
	"sync"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// RuleDialer struct
type RuleDialer struct {
	gDialer proxy.Dialer

	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewRuleDialer returns a new rule dialer
func NewRuleDialer(rules []*RuleConf, gDialer proxy.Dialer) *RuleDialer {
	rd := &RuleDialer{gDialer: gDialer}

	for _, r := range rules {
		var fwdrs []proxy.Dialer
		for _, chain := range r.Forward {
			var fwdr proxy.Dialer
			var err error
			for _, url := range strings.Split(chain, ",") {
				fwdr, err = proxy.DialerFromURL(url, fwdr)
				if err != nil {
					log.Fatal(err)
				}
			}
			fwdrs = append(fwdrs, fwdr)
		}

		sDialer := NewStrategyDialer(r.Strategy, fwdrs, r.CheckWebSite, r.CheckDuration)

		for _, domain := range r.Domain {
			rd.domainMap.Store(strings.ToLower(domain), sDialer)
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
func (rd *RuleDialer) Addr() string { return "RULE DIALER, DEFAULT: " + rd.gDialer.Addr() }

// NextDialer return next dialer according to rule
func (rd *RuleDialer) NextDialer(dstAddr string) proxy.Dialer {
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
			return dialer.(proxy.Dialer)
		}

		var ret proxy.Dialer
		// check cidr
		rd.cidrMap.Range(func(key, value interface{}) bool {
			cidr := key.(*net.IPNet)
			if cidr.Contains(ip) {
				ret = value.(proxy.Dialer)
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
			return dialer.(proxy.Dialer)
		}
	}

	return rd.gDialer
}

// Dial dials to targer addr and return a conn
func (rd *RuleDialer) Dial(network, addr string) (net.Conn, error) {
	return rd.NextDialer(addr).Dial(network, addr)
}

// DialUDP connects to the given address via the proxy
func (rd *RuleDialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return rd.NextDialer(addr).DialUDP(network, addr)
}

// AddDomainIP used to update ipMap rules according to domainMap rule
func (rd *RuleDialer) AddDomainIP(domain, ip string) error {
	if ip != "" {
		domainParts := strings.Split(domain, ".")
		length := len(domainParts)
		for i := length - 2; i >= 0; i-- {
			pDomain := strings.ToLower(strings.Join(domainParts[i:length], "."))

			// find in domainMap
			if dialer, ok := rd.domainMap.Load(pDomain); ok {
				rd.ipMap.Store(ip, dialer)
				log.F("rule add ip=%s, based on rule: domain=%s & domain/ip: %s/%s\n", ip, pDomain, domain, ip)
			}
		}

	}
	return nil
}
