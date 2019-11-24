// Package ws implements a simple websocket client.
package ws

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/dongxinb/glider/common/log"
	"github.com/dongxinb/glider/proxy"
)

// WS is the base ws proxy struct.
type WS struct {
	dialer proxy.Dialer
	addr   string

	client *Client
}

func init() {
	proxy.RegisterDialer("ws", NewWSDialer)
}

// NewWS returns a websocket proxy.
func NewWS(s string, d proxy.Dialer) (*WS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	// TODO:
	if addr == "" {
		addr = d.Addr()
	}

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]

	client, err := NewClient(serverName, u.Path)
	if err != nil {
		log.F("create ws client err: %s", err)
		return nil, err
	}

	p := &WS{
		dialer: d,
		addr:   addr,
		client: client,
	}

	return p, nil
}

// NewWSDialer returns a ws proxy dialer.
func NewWSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewWS(s, d)
}

// Addr returns forwarder's address.
func (s *WS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *WS) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	return s.client.NewConn(rc, addr)
}

// DialUDP connects to the given address via the proxy.
func (s *WS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("ws client does not support udp now")
}
