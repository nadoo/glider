// +build !linux

package main

import (
	"errors"
	"log"
)

// RedirProxy struct
type RedirProxy struct{}

// NewRedirProxy returns a redirect proxy.
func NewRedirProxy(addr string, sDialer Dialer) (*RedirProxy, error) {
	return nil, errors.New("redir not supported on this os")
}

// ListenAndServe .
func (s *RedirProxy) ListenAndServe() {
	log.Fatal("redir not supported on this os")
}
