// +build windows

package main

import "log"

type redir struct{ Proxy }

// RedirProxy returns a redirect proxy.
func RedirProxy(addr string, upProxy Proxy) (Proxy, error) {
	return &redir{Proxy: upProxy}, nil
}

// ListenAndServe redirected requests as a server.
func (s *redir) ListenAndServe() {
	log.Fatal("redir not supported on windows")
}
