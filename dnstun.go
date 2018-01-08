// https://tools.ietf.org/html/rfc1035

package main

// DNSTun struct
type DNSTun struct {
	*Forwarder        // as client
	sDialer    Dialer // dialer for server

	raddr string

	dns *DNS
	tcp *TCPTun
}

// NewDNSTun returns a dns tunnel forwarder.
func NewDNSTun(addr, raddr string, sDialer Dialer) (*DNSTun, error) {
	s := &DNSTun{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,

		raddr: raddr,
	}

	s.dns, _ = NewDNS(addr, raddr, sDialer, true)

	return s, nil
}

// ListenAndServe .
func (s *DNSTun) ListenAndServe() {
	if s.dns != nil {
		go s.dns.ListenAndServe()
	}
}
