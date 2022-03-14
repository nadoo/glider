// https://www.rfc-editor.org/rfc/rfc1928

// socks5 client:
// https://github.com/golang/net/tree/master/proxy
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socks5 implements a socks5 proxy.
package socks5

import (
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Version is socks5 version number.
const Version = 5

// Socks5 is a base socks5 struct.
type Socks5 struct {
	dialer   proxy.Dialer
	proxy    proxy.Proxy
	addr     string
	user     string
	password string
}

// NewSocks5 returns a Proxy that makes SOCKS v5 connections to the given address.
// with an optional username and password. (RFC 1928)
func NewSocks5(s string, d proxy.Dialer, p proxy.Proxy) (*Socks5, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()

	h := &Socks5{
		dialer:   d,
		proxy:    p,
		addr:     addr,
		user:     user,
		password: pass,
	}

	return h, nil
}

func init() {
	proxy.AddUsage("socks5", `
Socks5 scheme:
  socks5://[user:pass@]host:port
`)
}
