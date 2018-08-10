package tls

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// TLS .
type TLS struct {
	addr string

	serverName string
	skipVerify bool
}

// NewTLS returns a tls proxy.
func NewTLS(s string) (*TLS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	query := u.Query()
	skipVerify := query.Get("skipVerify")

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]

	p := &TLS{
		addr:       addr,
		serverName: serverName,
		skipVerify: false,
	}

	if skipVerify == "true" {
		p.skipVerify = true
	}

	return p, nil
}
