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

type ServerCreator func(s string, dialer Dialer) (Server, error)

var (
	serverMap = make(map[string]ServerCreator)
)

func RegisterServer(name string, c ServerCreator) {
	serverMap[name] = c
}

func ServerFromURL(s string, dialer Dialer) (Server, error) {
	if !strings.Contains(s, "://") {
		s = "mixed://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	if dialer == nil {
		dialer = Direct
	}

	c, ok := serverMap[strings.ToLower(u.Scheme)]
	if ok {
		return c(s, dialer)
	}

	return nil, errors.New("unknown scheme '" + u.Scheme + "'")
}
