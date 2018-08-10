package udptun

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// UDPTun struct
type UDPTun struct {
	addr  string
	raddr string
}

// NewUDPTun returns a UDPTun proxy.
func NewUDPTun(s string) (*UDPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &UDPTun{
		addr:  d[0],
		raddr: d[1],
	}

	return p, nil
}
