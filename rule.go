package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/nadoo/conflag"
)

// ruleForwarder, every ruleForwarder points to a rule file
type ruleForwarder struct {
	Forward       arrFlags
	Strategy      string
	CheckWebSite  string
	CheckDuration int

	Domain arrFlags
	IP     arrFlags
	CIDR   arrFlags

	name       string
	sForwarder Proxy
}

// newRuleProxyFromFile .
func newRuleProxyFromFile(ruleFile string) (*ruleForwarder, error) {
	p := &ruleForwarder{name: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.Var(&p.Forward, "forward", "forward url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT[,SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT]")
	f.StringVar(&p.Strategy, "strategy", "rr", "forward strategy, default: rr")
	f.StringVar(&p.CheckWebSite, "checkwebsite", "www.apple.com:443", "proxy check website address")
	f.IntVar(&p.CheckDuration, "checkduration", 30, "proxy check duration(seconds)")

	f.Var(&p.Domain, "domain", "domain")
	f.Var(&p.IP, "ip", "ip")
	f.Var(&p.CIDR, "cidr", "cidr")

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

	p.sForwarder = forwarder

	return p, err
}

func (p *ruleForwarder) Addr() string        { return "rule forwarder" }
func (p *ruleForwarder) ListenAndServe()     {}
func (p *ruleForwarder) Serve(c net.Conn)    {}
func (p *ruleForwarder) CurrentProxy() Proxy { return p.sForwarder.CurrentProxy() }

func (p *ruleForwarder) GetProxy(dstAddr string) Proxy {

	return p.sForwarder.NextProxy()
}

func (p *ruleForwarder) NextProxy() Proxy {
	return p.sForwarder.NextProxy()
}

func (p *ruleForwarder) Enabled() bool         { return true }
func (p *ruleForwarder) SetEnable(enable bool) {}

func (p *ruleForwarder) Dial(network, addr string) (net.Conn, error) {
	return p.NextProxy().Dial(network, addr)
}
