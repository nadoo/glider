package proxy

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/dongxinb/glider/common/log"
)

// Dialer is used to create connection.
type Dialer interface {
	// Addr is the dialer's addr
	Addr() string

	// Dial connects to the given address
	Dial(network, addr string) (c net.Conn, err error)

	// DialUDP connects to the given address
	DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error)
}

// DialerCreator is a function to create dialers.
type DialerCreator func(s string, dialer Dialer) (Dialer, error)

var (
	dialerMap = make(map[string]DialerCreator)
)

// RegisterDialer is used to register a dialer.
func RegisterDialer(name string, c DialerCreator) {
	dialerMap[name] = c
}

// DialerFromURL calls the registered creator to create dialers.
// dialer is the default upstream dialer so cannot be nil, we can use Default when calling this function.
func DialerFromURL(s string, dialer Dialer) (Dialer, error) {
	if dialer == nil {
		return nil, errors.New("DialerFromURL: dialer cannot be nil")
	}

	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	c, ok := dialerMap[strings.ToLower(u.Scheme)]
	if ok {
		return c(s, dialer)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
