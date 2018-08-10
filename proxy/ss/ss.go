package ss

import (
	"net/url"
	"strings"

	"github.com/shadowsocks/go-shadowsocks2/core"

	"github.com/nadoo/glider/common/log"
)

// SS .
type SS struct {
	addr string
	core.Cipher
}

// NewSS returns a shadowsocks proxy.
func NewSS(s string) (*SS, error) {
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

	p := &SS{
		addr:   addr,
		Cipher: ciph,
	}

	return p, nil
}

// ListCipher .
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}
