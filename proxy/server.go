package proxy

import (
	"errors"
	"net"
	"strings"
)

// Server interface.
type Server interface {
	// ListenAndServe sets up a listener and serve on it
	ListenAndServe()

	// Serve serves a connection
	Serve(c net.Conn)
}

// ServerCreator is a function to create proxy servers.
type ServerCreator func(s string, proxy Proxy) (Server, error)

var (
	serverCreators = make(map[string]ServerCreator)
)

// RegisterServer is used to register a proxy server.
func RegisterServer(name string, c ServerCreator) {
	serverCreators[strings.ToLower(name)] = c
}

// ServerFromURL calls the registered creator to create proxy servers.
// dialer is the default upstream dialer so cannot be nil, we can use Default when calling this function.
func ServerFromURL(s string, proxy Proxy) (Server, error) {
	if proxy == nil {
		return nil, errors.New("ServerFromURL: dialer cannot be nil")
	}

	if !strings.Contains(s, "://") {
		s = "mixed://" + s
	}

	scheme := s[:strings.Index(s, ":")]
	c, ok := serverCreators[strings.ToLower(scheme)]
	if ok {
		return c(s, proxy)
	}

	return nil, errors.New("unknown scheme '" + scheme + "'")
}
