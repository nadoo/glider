// https://tools.ietf.org/html/rfc1035

package main

// DNSTun .
type DNSTun struct {
	*Forwarder        // as client
	sDialer    Dialer // dialer for server

	raddr string

	udp *DNS
	tcp *TCPTun
}

// NewDNSTun returns a dns forwarder.
func NewDNSTun(addr, raddr string, sDialer Dialer) (*DNSTun, error) {
	s := &DNSTun{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,

		raddr: raddr,
	}

	s.udp, _ = NewDNS(addr, raddr, sDialer)
	s.tcp, _ = NewTCPTun(addr, raddr, sDialer)

	return s, nil
}

// ListenAndServe .
func (s *DNSTun) ListenAndServe() {
	if s.udp != nil {
		go s.udp.ListenAndServe()
	}

	if s.tcp != nil {
		s.tcp.ListenAndServe()
	}
}
