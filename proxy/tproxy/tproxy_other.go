// +build !linux

package main

import (
	"errors"
	"log"
)

// TProxy struct
type TProxy struct{}

// NewTProxy returns a tproxy.
func NewTProxy(addr string, sDialer Dialer) (*TProxy, error) {
	return nil, errors.New("tproxy not supported on this os")
}

// ListenAndServe .
func (s *TProxy) ListenAndServe() {
	log.Fatal("tproxy not supported on this os")
}
