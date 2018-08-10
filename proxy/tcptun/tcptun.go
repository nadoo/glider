package tcptun

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// TCPTun struct
type TCPTun struct {
	addr  string
	raddr string
}

// NewTCPTun returns a tcptun proxy.
func NewTCPTun(s string) (*TCPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &TCPTun{
		addr:  d[0],
		raddr: d[1],
	}

	return p, nil
}
