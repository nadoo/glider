package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/nadoo/conflag"
)

// ruleForwarder, every ruleForwarder points to a rule file
type ruleForwarder struct {
	Forward       []string
	Strategy      string
	CheckWebSite  string
	CheckDuration int

	DNSServer []string
	IPSet     string

	Domain []string
	IP     []string
	CIDR   []string

	name string
	Proxy
}

// newRuleProxyFromFile .
func newRuleProxyFromFile(ruleFile string) (*ruleForwarder, error) {
	p := &ruleForwarder{name: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.StringSliceUniqVar(&p.Forward, "forward", nil, "forward url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT[,SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT]")
	f.StringVar(&p.Strategy, "strategy", "rr", "forward strategy, default: rr")
	f.StringVar(&p.CheckWebSite, "checkwebsite", "www.apple.com", "proxy check HTTP(NOT HTTPS) website address, format: HOST[:PORT], default port: 80")
	f.IntVar(&p.CheckDuration, "checkduration", 30, "proxy check duration(seconds)")

	f.StringSliceUniqVar(&p.DNSServer, "dnsserver", nil, "remote dns server")
	f.StringVar(&p.IPSet, "ipset", "", "ipset name")

	f.StringSliceUniqVar(&p.Domain, "domain", nil, "domain")
	f.StringSliceUniqVar(&p.IP, "ip", nil, "ip")
	f.StringSliceUniqVar(&p.CIDR, "cidr", nil, "cidr")

	err := f.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		return nil, err
	}

	var forwarders []Proxy
	for _, chain := range p.Forward {
		var forward Proxy
		var err error
		for _, url := range strings.Split(chain, ",") {
			forward, err = ProxyFromURL(url, forward)
			if err != nil {
				log.Fatal(err)
			}
		}
		forwarders = append(forwarders, forward)
	}

	forwarder := newStrategyForwarder(p.Strategy, forwarders)

	for _, forward := range forwarders {
		go check(forward, p.CheckWebSite, p.CheckDuration)
	}

	p.Proxy = forwarder

	return p, err
}
