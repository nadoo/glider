package proxy

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// Dialer means to establish a connection and relay it.
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

// DialerCreator is a function to create dialers.
type DialerCreator func(s string, dialer Dialer) (Dialer, error)

var (
	dialerMap = make(map[string]DialerCreator)
)

// RegisterDialer is used to register a dialer
func RegisterDialer(name string, c DialerCreator) {
	dialerMap[name] = c
}

// DialerFromURL calls the registered creator to create dialers.
func DialerFromURL(s string, dialer Dialer) (Dialer, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	if dialer == nil {
		dialer = Direct
	}

	c, ok := dialerMap[strings.ToLower(u.Scheme)]
	if ok {
		return c(s, dialer)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
