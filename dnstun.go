// https://tools.ietf.org/html/rfc1035

package main

// DNSTun struct
type DNSTun struct {
	dialer Dialer
	addr   string

	raddr string

	dns *DNS
	tcp *TCPTun
}

// NewDNSTun returns a dns tunnel forwarder.
func NewDNSTun(addr, raddr string, dialer Dialer) (*DNSTun, error) {
	s := &DNSTun{
		dialer: dialer,
		addr:   addr,

		raddr: raddr,
	}

	s.dns, _ = NewDNS(addr, raddr, dialer, true)

	return s, nil
}

// ListenAndServe .
func (s *DNSTun) ListenAndServe() {
	if s.dns != nil {
		go s.dns.ListenAndServe()
	}
}
