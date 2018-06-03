package main

import (
	"errors"
	"net/url"
	"strings"
)

// Server interface
type Server interface {
	// ListenAndServe as proxy server, use only in server mode.
	ListenAndServe()
}

// ServerFromURL parses url and get a Proxy
// TODO: table
func ServerFromURL(s string, dialer Dialer) (Server, error) {
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

	if dialer == nil {
		dialer = Direct
	}

	switch u.Scheme {
	case "mixed":
		return NewMixedProxy(addr, user, pass, u.RawQuery, dialer)
	case "http":
		return NewHTTP(addr, user, pass, u.RawQuery, dialer)
	case "socks5":
		return NewSOCKS5(addr, user, pass, dialer)
	case "ss":
		return NewSS(addr, user, pass, dialer)
	case "redir":
		return NewRedirProxy(addr, dialer)
	case "tcptun":
		d := strings.Split(addr, "=")
		return NewTCPTun(d[0], d[1], dialer)
	case "udptun":
		d := strings.Split(addr, "=")
		return NewUDPTun(d[0], d[1], dialer)
	case "dnstun":
		d := strings.Split(addr, "=")
		return NewDNSTun(d[0], d[1], dialer)
	case "uottun":
		d := strings.Split(addr, "=")
		return NewUoTTun(d[0], d[1], dialer)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
