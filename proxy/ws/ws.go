package ws

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// WS .
type WS struct {
	addr   string
	client *Client
}

// NewWS returns a websocket proxy.
func NewWS(s string, dialer proxy.Dialer) (*WS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	// TODO:
	if addr == "" {
		addr = dialer.Addr()
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
		addr:   addr,
		client: client,
	}

	return p, nil
}
