// https://tools.ietf.org/html/rfc1035

package main

type DNSTun struct {
	*proxy
	raddr string

	udp Proxy
	tcp Proxy
}

// NewDNSTun returns a dns forwarder.
func NewDNSTun(addr, raddr string, upProxy Proxy) (*DNSTun, error) {
	s := &DNSTun{
		proxy: NewProxy(addr, upProxy),
		raddr: raddr,
	}

	s.udp, _ = NewDNS(addr, raddr, upProxy)
	s.tcp, _ = NewTCPTun(addr, raddr, upProxy)

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
