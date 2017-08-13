// https://tools.ietf.org/html/rfc1035

package main

type dnstun struct {
	*proxy
	raddr string

	udp Proxy
	tcp Proxy
}

// DNSTun returns a dns forwarder. client -> dns.udp -> glider -> forwarder -> remote dns addr
func DNSTun(addr, raddr string, upProxy Proxy) (Proxy, error) {
	s := &dnstun{
		proxy: newProxy(addr, upProxy),
		raddr: raddr,
	}

	s.udp, _ = DNSForwarder(addr, raddr, upProxy)
	s.tcp, _ = TCPTun(addr, raddr, upProxy)

	return s, nil
}

// ListenAndServe .
func (s *dnstun) ListenAndServe() {
	if s.udp != nil {
		go s.udp.ListenAndServe()
	}

	if s.tcp != nil {
		s.tcp.ListenAndServe()
	}
}
