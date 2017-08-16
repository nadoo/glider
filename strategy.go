package main

import "net"

// NewStrategyForwarder .
func NewStrategyForwarder(strategy string, forwarders []Proxy) Proxy {
	var proxy Proxy
	if len(forwarders) == 0 {
		proxy = Direct
	} else if len(forwarders) == 1 {
		proxy = forwarders[0]
	} else if len(forwarders) > 1 {
		switch strategy {
		case "rr":
			proxy = newRRProxy("", forwarders)
			logf("forward to remote servers in round robin mode.")
		case "ha":
			proxy = newHAProxy("", forwarders)
			logf("forward to remote servers in high availability mode.")
		default:
			logf("not supported forward mode '%s', just use the first forward server.", conf.Strategy)
			proxy = forwarders[0]
		}
	}

	return proxy
}

// rrProxy
type rrProxy struct {
	forwarders []Proxy
	idx        int
}

// newRRProxy .
func newRRProxy(addr string, forwarders []Proxy) Proxy {
	if len(forwarders) == 0 {
		return Direct
	} else if len(forwarders) == 1 {
		return NewProxy(addr, forwarders[0])
	}

	return &rrProxy{forwarders: forwarders}
}

func (p *rrProxy) Addr() string                  { return "strategy forwarder" }
func (p *rrProxy) ListenAndServe()               {}
func (p *rrProxy) Serve(c net.Conn)              {}
func (p *rrProxy) CurrentProxy() Proxy           { return p.forwarders[p.idx] }
func (p *rrProxy) GetProxy(dstAddr string) Proxy { return p.NextProxy() }

func (p *rrProxy) NextProxy() Proxy {
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

func (p *rrProxy) Enabled() bool         { return true }
func (p *rrProxy) SetEnable(enable bool) {}

func (p *rrProxy) Dial(network, addr string) (net.Conn, error) {
	return p.GetProxy(addr).Dial(network, addr)
}

// high availability proxy
type haProxy struct {
	Proxy
}

// newHAProxy .
func newHAProxy(addr string, forwarders []Proxy) Proxy {
	return &haProxy{Proxy: newRRProxy(addr, forwarders)}
}

func (p *haProxy) GetProxy(dstAddr string) Proxy {
	proxy := p.CurrentProxy()
	if proxy.Enabled() == false {
		return p.NextProxy()
	}
	return proxy
}

func (p *haProxy) Dial(network, addr string) (net.Conn, error) {
	return p.GetProxy(addr).Dial(network, addr)
}
