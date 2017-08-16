// +build !linux

package main

import "log"

type RedirProxy struct{ *proxy }

// NewRedirProxy returns a redirect proxy.
func NewRedirProxy(addr string, upProxy Proxy) (Proxy, error) {
	return &RedirProxy{proxy: NewProxy(addr, upProxy)}, nil
}

// ListenAndServe redirected requests as a server.
func (s *RedirProxy) ListenAndServe() {
	log.Fatal("redir not supported on this os")
}
