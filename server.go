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
func ServerFromURL(s string, sDialer Dialer) (Server, error) {
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

	if sDialer == nil {
		sDialer = Direct
	}

	switch u.Scheme {
	case "mixed":
		return NewMixedProxy(addr, user, pass, sDialer)
	case "http":
		return NewHTTP(addr, nil, sDialer)
	case "socks5":
		return NewSOCKS5(addr, user, pass, nil, sDialer)
	case "ss":
		p, err := NewSS(addr, user, pass, nil, sDialer)
		return p, err
	case "redir":
		return NewRedirProxy(addr, sDialer)
	case "tcptun":
		d := strings.Split(addr, "=")
		return NewTCPTun(d[0], d[1], sDialer)
	case "dnstun":
		d := strings.Split(addr, "=")
		return NewDNSTun(d[0], d[1], sDialer)
	case "uottun":
		d := strings.Split(addr, "=")
		return NewUoTTun(d[0], d[1], sDialer)
	}

	return nil, errors.New("unknown schema '" + u.Scheme + "'")
}
