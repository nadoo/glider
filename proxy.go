package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

// A Proxy means to establish a connection and relay it.
type Proxy interface {
	// Get address
	Addr() string

	// ListenAndServe as proxy server, use only in server mode.
	ListenAndServe()

	// Serve as proxy server, use only in server mode.
	Serve(c net.Conn)

	// Get current proxy
	CurrentProxy() Proxy

	// Get a proxy based on the destAddr and strategy
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

func (p *proxy) Addr() string                  { return p.addr }
func (p *proxy) ListenAndServe()               { logf("base proxy ListenAndServe") }
func (p *proxy) Serve(c net.Conn)              { logf("base proxy Serve") }
func (p *proxy) CurrentProxy() Proxy           { return p.forward }
func (p *proxy) GetProxy(dstAddr string) Proxy { return p.forward }
func (p *proxy) NextProxy() Proxy              { return p.forward }
func (p *proxy) Enabled() bool                 { return p.enabled }
func (p *proxy) SetEnable(enable bool)         { p.enabled = enable }

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
	case "mixed":
		return MixedProxy("tcp", addr, user, pass, forwarder)
	case "http":
		return HTTPProxy(addr, forwarder)
	case "socks5":
		return SOCKS5Proxy("tcp", addr, user, pass, forwarder)
	case "ss":
		p, err := SSProxy(addr, user, pass, forwarder)
		return p, err
	case "redir":
		return RedirProxy(addr, forwarder)
	case "tcptun":
		d := strings.Split(addr, "=")
		return TCPTun(d[0], d[1], forwarder)
	case "dnstun":
		d := strings.Split(addr, "=")
		return DNSTun(d[0], d[1], forwarder)
	}

	return nil, errors.New("unknown schema '" + u.Scheme + "'")
}

// Check proxy
func check(p Proxy, webhost string, duration int) {
	retry := 1
	buf := make([]byte, 4)

	if strings.IndexByte(webhost, ':') == -1 {
		webhost = webhost + ":80"
	}

	for {
		time.Sleep(time.Duration(duration) * time.Second * time.Duration(retry>>1))
		retry <<= 1

		if retry > 16 {
			retry = 16
		}

		startTime := time.Now()
		c, err := p.Dial("tcp", webhost)
		if err != nil {
			p.SetEnable(false)
			logf("proxy-check %s -> %s, set to DISABLED. error in dial: %s", p.Addr(), webhost, err)
			continue
		}

		c.Write([]byte("GET / HTTP/1.0"))
		c.Write([]byte("\r\n\r\n"))

		_, err = io.ReadFull(c, buf)
		if err != nil {
			p.SetEnable(false)
			logf("proxy-check %s -> %s, set to DISABLED. error in read: %s", p.Addr(), webhost, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			p.SetEnable(true)
			retry = 2
			dialTime := time.Since(startTime)
			logf("proxy-check %s -> %s, set to ENABLED. connect time: %s", p.Addr(), webhost, dialTime.String())
		} else {
			p.SetEnable(false)
			logf("proxy-check %s -> %s, set to DISABLED. server response: %s", p.Addr(), webhost, buf)
		}

		c.Close()
	}
}
