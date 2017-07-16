package main

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// A Proxy means to establish a connection and relay it.
type Proxy interface {
	// ListenAndServe as proxy server, use only in server mode.
	ListenAndServe()

	// Serve as proxy server, use only in server mode.
	Serve(c net.Conn)

	// Get address
	Addr() string

	// Get current proxy
	CurrentProxy() Proxy

	// Get a proxy according to the strategy
	GetProxy() Proxy

	// Switch to the next proxy
	NextProxy() Proxy

	// Get the status of proxy
	Enabled() bool

	// Set the proxy status
	SetEnable(enable bool)

	// Dial connects to the given address via the proxy.
	Dial(network, addr string) (c net.Conn, err error)
}

// proxy
type proxy struct {
	addr    string
	forward Proxy
	enabled bool
}

// newProxy .
func newProxy(addr string, forward Proxy) Proxy {
	if forward == nil {
		forward = Direct
	}

	return &proxy{addr: addr, forward: forward, enabled: false}
}

func (p *proxy) ListenAndServe()       { logf("base proxy ListenAndServe") }
func (p *proxy) Serve(c net.Conn)      { logf("base proxy Serve") }
func (p *proxy) CurrentProxy() Proxy   { return p.forward }
func (p *proxy) GetProxy() Proxy       { return p.forward }
func (p *proxy) NextProxy() Proxy      { return p.forward }
func (p *proxy) Enabled() bool         { return p.enabled }
func (p *proxy) SetEnable(enable bool) { p.enabled = enable }
func (p *proxy) Addr() string          { return p.addr }

func (p *proxy) Dial(network, addr string) (net.Conn, error) {
	return p.forward.Dial(network, addr)
}

// ProxyFromURL parses url and get a Proxy
// TODO: table
func ProxyFromURL(s string, forwarders ...Proxy) (Proxy, error) {
	if !strings.Contains(s, "://") {
		s = "mixed://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		logf("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}

	var proxy Proxy
	if len(forwarders) == 0 {
		proxy = newProxy(addr, Direct)
	} else if len(forwarders) == 1 {
		proxy = newProxy(addr, forwarders[0])
	} else if len(forwarders) > 1 {
		switch config.Strategy {
		case "rr":
			proxy = newRRProxy(addr, forwarders)
			logf("forward to remote servers in round robin mode.")
		case "ha":
			proxy = newHAProxy(addr, forwarders)
			logf("forward to remote servers in high availability mode.")
		default:
			logf("not supported forward mode '%s', just use the first forward server.", config.Strategy)
			proxy = newProxy(addr, forwarders[0])
		}
	}

	switch u.Scheme {
	case "ss":
		p, err := SSProxy(user, pass, proxy)
		return p, err
	case "socks5":
		return SOCKS5Proxy("tcp", addr, user, pass, proxy)
	case "redir":
		return RedirProxy(addr, proxy)
	case "tcptun":
		d := strings.Split(addr, "=")
		return TCPTunProxy(d[0], d[1], proxy)
	case "dnstun":
		d := strings.Split(addr, "=")
		return DNSTunProxy(d[0], d[1], proxy)
	case "http":
		return HTTPProxy(addr, proxy)
	case "mixed":
		return MixedProxy("tcp", addr, user, pass, proxy)
	}

	return nil, errors.New("unknown schema '" + u.Scheme + "'")
}

// Check proxy
func check(p Proxy, target string, duration int) {
	firstTime := true
	for {
		if !firstTime {
			time.Sleep(time.Duration(duration) * time.Second)
		}
		firstTime = false

		startTime := time.Now()
		c, err := p.Dial("tcp", target)
		if err != nil {
			logf("proxy-check %s -> %s, set to DISABLED. error: %s", p.Addr(), config.CheckSite, err)
			p.SetEnable(false)
			continue
		}
		c.Close()
		p.SetEnable(true)

		// TODO: choose the fastest proxy.
		dialTime := time.Since(startTime)
		logf("proxy-check: %s -> %s, connect time: %s", p.Addr(), config.CheckSite, dialTime.String())
	}
}
