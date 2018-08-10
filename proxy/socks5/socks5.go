// https://tools.ietf.org/html/rfc1928

// socks5 client:
// https://github.com/golang/net/tree/master/proxy
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// socks5 server:
// https://github.com/shadowsocks/go-shadowsocks2/tree/master/socks

package socks5

import (
	"net/url"

	"github.com/nadoo/glider/common/log"
)

// Version is socks5 version number
const Version = 5

// SOCKS5 struct
type SOCKS5 struct {
	addr     string
	user     string
	password string
}

// NewSOCKS5 returns a Proxy that makes SOCKS v5 connections to the given address
// with an optional username and password. See RFC 1928.
func NewSOCKS5(s string) (*SOCKS5, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()

	h := &SOCKS5{
		addr:     addr,
		user:     user,
		password: pass,
	}

	return h, nil
}
