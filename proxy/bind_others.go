//go:build !linux
// +build !linux

package proxy

import "net"

func bind(dialer *net.Dialer, iface *net.Interface) {}
