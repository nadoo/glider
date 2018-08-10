package proxy

import (
	"errors"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// Server interface
type Server interface {
	// ListenAndServe as proxy server, use only in server mode.
	ListenAndServe()
}

// ServerCreator is a function to create proxy servers.
type ServerCreator func(s string, f *Forwarder) (Server, error)

var (
	serverMap = make(map[string]ServerCreator)
)

// RegisterServer is used to register a proxy server
func RegisterServer(name string, c ServerCreator) {
	serverMap[name] = c
}

// ServerFromURL calls the registered creator to create proxy servers.
func ServerFromURL(s string, f *Forwarder) (Server, error) {
	if !strings.Contains(s, "://") {
		s = "mixed://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	if f == nil {
		f = NewForwarder(Direct)
	}

	c, ok := serverMap[strings.ToLower(u.Scheme)]
	if ok {
		return c(s, f)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
