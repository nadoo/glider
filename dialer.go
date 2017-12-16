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

	// Get the dialer by dstAddr
	NextDialer(dstAddr string) Dialer
}

// DialerFromURL parses url and get a Proxy
// TODO: table
func DialerFromURL(s string, cDialer Dialer) (Dialer, error) {
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

	if cDialer == nil {
		cDialer = Direct
	}

	switch u.Scheme {
	case "http":
		return NewHTTP(addr, user, pass, cDialer, nil)
	case "socks5":
		return NewSOCKS5(addr, user, pass, cDialer, nil)
	case "ss":
		p, err := NewSS(addr, user, pass, cDialer, nil)
		return p, err
	}

	return nil, errors.New("unknown schema '" + u.Scheme + "'")
}
