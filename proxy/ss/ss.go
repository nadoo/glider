package ss

import (
	"net/url"
	"strings"

	"github.com/nadoo/go-shadowsocks2/core"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// SS is a base ss struct.
type SS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	core.Cipher
}

func init() {
	proxy.RegisterDialer("ss", NewSSDialer)
	proxy.RegisterServer("ss", NewSSServer)
}

// NewSS returns a ss proxy.
func NewSS(s string, d proxy.Dialer, p proxy.Proxy) (*SS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	method := u.User.Username()
	pass, _ := u.User.Password()

	ciph, err := core.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("[ss] PickCipher for '%s', error: %s", method, err)
	}

	ss := &SS{
		dialer: d,
		proxy:  p,
		addr:   addr,
		Cipher: ciph,
	}

	return ss, nil
}

// ListCipher returns all the ciphers supported.
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}
