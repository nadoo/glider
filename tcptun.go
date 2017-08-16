package main

import "net"

type TCPTun struct {
	*proxy
	raddr string
}

// NewTCPTun returns a redirect proxy.
func NewTCPTun(addr, raddr string, upProxy Proxy) (*TCPTun, error) {
	s := &TCPTun{
		proxy: NewProxy(addr, upProxy),
		raddr: raddr,
	}

	return s, nil
}

// ListenAndServe redirected requests as a server.
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

			rc, err := s.GetProxy(s.raddr).Dial("tcp", s.raddr)
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
