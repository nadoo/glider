// +build !linux

package main

import "log"

type redir struct{ *proxy }

// RedirProxy returns a redirect proxy.
func RedirProxy(addr string, upProxy Proxy) (Proxy, error) {
	return &redir{proxy: newProxy(addr, upProxy)}, nil
}

// ListenAndServe redirected requests as a server.
func (s *redir) ListenAndServe() {
	log.Fatal("redir not supported on this os")
}
