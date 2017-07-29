package main

import (
	"errors"
	"io"
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
	GetProxy(dstAddr string) Proxy

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
func newProxy(addr string, forward Proxy) *proxy {
	if forward == nil {
		forward = Direct
	}

	return &proxy{addr: addr, forward: forward, enabled: true}
}

func (p *proxy) ListenAndServe()               { logf("base proxy ListenAndServe") }
func (p *proxy) Serve(c net.Conn)              { logf("base proxy Serve") }
func (p *proxy) CurrentProxy() Proxy           { return p.forward }
func (p *proxy) GetProxy(dstAddr string) Proxy { return p.forward }
func (p *proxy) NextProxy() Proxy              { return p.forward }
func (p *proxy) Enabled() bool                 { return p.enabled }
func (p *proxy) SetEnable(enable bool)         { p.enabled = enable }
func (p *proxy) Addr() string                  { return p.addr }

func (p *proxy) Dial(network, addr string) (net.Conn, error) {
	return p.forward.Dial(network, addr)
}

// ProxyFromURL parses url and get a Proxy
// TODO: table
func ProxyFromURL(s string, forwarder Proxy) (Proxy, error) {
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

	switch u.Scheme {
	case "ss":
		p, err := SSProxy(addr, user, pass, forwarder)
		return p, err
	case "socks5":
		return SOCKS5Proxy("tcp", addr, user, pass, forwarder)
	case "redir":
		return RedirProxy(addr, forwarder)
	case "tcptun":
		d := strings.Split(addr, "=")
		return TCPTunProxy(d[0], d[1], forwarder)
	case "dnstun":
		d := strings.Split(addr, "=")
		return DNSTunProxy(d[0], d[1], forwarder)
	case "http":
		return HTTPProxy(addr, forwarder)
	case "mixed":
		return MixedProxy("tcp", addr, user, pass, forwarder)
	}

	return nil, errors.New("unknown schema '" + u.Scheme + "'")
}

// Check proxy
func check(p Proxy, target string, duration int) {
	firstTime := true
	buf := make([]byte, 8)

	for {
		if !firstTime {
			time.Sleep(time.Duration(duration) * time.Second)
		}
		firstTime = false

		startTime := time.Now()
		c, err := p.Dial("tcp", target)
		if err != nil {
			p.SetEnable(false)
			logf("proxy-check %s -> %s, set to DISABLED. error: %s", p.Addr(), target, err)
			continue
		}

		c.Write([]byte("GET / HTTP/1.0"))
		c.Write([]byte("\r\n\r\n"))

		_, err = c.Read(buf)
		if err != nil && err != io.EOF {
			p.SetEnable(false)
			logf("proxy-check %s -> %s, set to DISABLED. error: %s", p.Addr(), target, err)
		} else {
			p.SetEnable(true)
			dialTime := time.Since(startTime)
			logf("proxy-check: %s -> %s, connect time: %s", p.Addr(), target, dialTime.String())
		}

		c.Close()
	}
}
