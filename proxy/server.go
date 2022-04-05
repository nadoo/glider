package proxy

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// Server interface.
type Server interface {
	// ListenAndServe sets up a listener and serve on it
	ListenAndServe()

	// Serve serves a connection
	Serve(c net.Conn)
}

// PacketServer interface.
type PacketServer interface {
	ServePacket(pc net.PacketConn)
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
// proxy can not be nil.
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

// ServerSchemes returns the registered server schemes.
func ServerSchemes() string {
	s := make([]string, 0, len(serverCreators))
	for name := range serverCreators {
		s = append(s, name)
	}
	sort.Strings(s)
	return strings.Join(s, " ")
}
