package main

import (
	"net"
	"time"
)

// strategyProxy
type strategyProxy struct {
	addr       string
	forwarders []Proxy
	idx        int
}

// newStrategyProxy .
func newStrategyProxy(addr string, forwarders []Proxy) Proxy {
	if len(forwarders) == 0 {
		return Direct
	} else if len(forwarders) == 1 {
		return newProxy(addr, forwarders[0])
	}

	for _, forward := range forwarders {
		go check(forward, config.CheckSite, config.CheckDuration)
	}

	return &strategyProxy{addr: addr, forwarders: forwarders}
}

func (p *strategyProxy) ListenAndServe()     {}
func (p *strategyProxy) Serve(c net.Conn)    {}
func (p *strategyProxy) CurrentProxy() Proxy { return p.forwarders[p.idx] }
func (p *strategyProxy) GetProxy() Proxy     { return p.NextProxy() }

func (p *strategyProxy) NextProxy() Proxy {
	n := len(p.forwarders)
	if n == 1 {
		return p.forwarders[0]
	}

	found := false
	for i := 0; i < n; i++ {
		p.idx = (p.idx + 1) % n
		if p.forwarders[p.idx].Enabled() {
			found = true
			break
		}
	}

	if !found {
		logf("NO AVAILABLE PROXY FOUND! please check your network or proxy server settings.")
	}

	return p.forwarders[p.idx]
}

func (p *strategyProxy) Enabled() bool         { return true }
func (p *strategyProxy) SetEnable(enable bool) {}

func (p *strategyProxy) Check(proxy Proxy, target string, duration time.Duration) {}

func (p *strategyProxy) Addr() string { return p.addr }

func (p *strategyProxy) Dial(network, addr string) (net.Conn, error) {
	return p.NextProxy().Dial(network, addr)
}

// round robin proxy
type rrproxy struct {
	Proxy
}

// newRRProxy .
func newRRProxy(addr string, forwarders []Proxy) Proxy {
	return newStrategyProxy(addr, forwarders)
}

// high availability proxy
type haproxy struct {
	Proxy
}

// newHAProxy .
func newHAProxy(addr string, forwarders []Proxy) Proxy {
	return &haproxy{Proxy: newStrategyProxy(addr, forwarders)}
}

func (p *haproxy) GetProxy() Proxy {
	proxy := p.CurrentProxy()
	if proxy.Enabled() == false {
		return p.NextProxy()
	}
	return proxy
}
