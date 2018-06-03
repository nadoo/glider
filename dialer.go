package main

import (
	"errors"
	"net"
	"net/url"
)

// A Dialer means to establish a connection and relay it.
type Dialer interface {
	// Addr()
	Addr() string

	// Dial connects to the given address via the proxy.
	Dial(network, addr string) (c net.Conn, err error)

	// DialUDP connects to the given address via the proxy.
	DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error)

	// Get the dialer by dstAddr
	NextDialer(dstAddr string) Dialer
}

// DialerFromURL parses url and get a Proxy
// TODO: table
func DialerFromURL(s string, dialer Dialer) (Dialer, error) {
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

	if dialer == nil {
		dialer = Direct
	}

	switch u.Scheme {
	case "http":
		return NewHTTP(addr, user, pass, "", dialer)
	case "socks5":
		return NewSOCKS5(addr, user, pass, dialer)
	case "ss":
		return NewSS(addr, user, pass, dialer)
	case "ssr":
		return NewSSR(addr, user, pass, u.RawQuery, dialer)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
