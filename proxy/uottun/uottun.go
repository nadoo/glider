package uottun

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// UoTTun udp over tcp tunnel
type UoTTun struct {
	addr  string
	raddr string
}

// NewUoTTun returns a UoTTun proxy.
func NewUoTTun(s string) (*UoTTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &UoTTun{
		addr:  d[0],
		raddr: d[1],
	}

	return p, nil
}
