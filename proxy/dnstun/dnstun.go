// https://tools.ietf.org/html/rfc1035

package dnstun

import (
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/tcptun"
)

// DNSTun struct
type DNSTun struct {
	dialer proxy.Dialer
	addr   string

	raddr string

	dns *dns.DNS
	tcp *tcptun.TCPTun
}

func init() {
	proxy.RegisterServer("dnstun", NewDNSTunServer)
}

// NewDNSTun returns a dns tunnel forwarder.
func NewDNSTun(s string, dialer proxy.Dialer) (*DNSTun, error) {

	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	addr, raddr := d[0], d[1]

	p := &DNSTun{
		dialer: dialer,
		addr:   addr,
		raddr:  raddr,
	}

	p.dns, _ = dns.NewDNS(addr, raddr, dialer, true)

	return p, nil
}

// NewDNSTunServer returns a dns tunnel server.
func NewDNSTunServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewDNSTun(s, dialer)
}

// ListenAndServe .
func (s *DNSTun) ListenAndServe() {
	if s.dns != nil {
		go s.dns.ListenAndServe()
	}
}
