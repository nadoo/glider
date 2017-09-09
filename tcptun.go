package main

import "net"

// TCPTun .
type TCPTun struct {
	*Forwarder
	sDialer Dialer

	raddr string
}

// NewTCPTun returns a tcptun proxy.
func NewTCPTun(addr, raddr string, sDialer Dialer) (*TCPTun, error) {
	s := &TCPTun{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,
		raddr:     raddr,
	}

	return s, nil
}

// ListenAndServe .
func (s *TCPTun) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}

	logf("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			rc, err := s.sDialer.Dial("tcp", s.raddr)
			if err != nil {

				logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			logf("proxy-tcptun %s <-> %s", c.RemoteAddr(), s.raddr)

			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}

		}()
	}
}
