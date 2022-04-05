package vsock

import (
	"net"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("vsock", NewVSockServer)
}

// NewVSockServer returns a vm socket server.
func NewVSockServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	vsock, err := NewVSock(schemes[0], nil, p)
	if err != nil {
		return nil, err
	}

	if len(schemes) > 1 {
		vsock.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	if vsock.cid == 0 {
		cid, err := ContextID()
		if err != nil {
			return nil, err
		}
		vsock.cid = cid
	}

	return vsock, nil
}

// ListenAndServe serves requests.
func (s *vsock) ListenAndServe() {
	l, err := Listen(s.cid, s.port)
	if err != nil {
		log.Fatalf("[vsock] failed to listen: %v", err)
		return
	}
	defer l.Close()

	log.F("[vsock] Listening on %s", l.Addr())

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[vsock] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests.
func (s *vsock) Serve(c net.Conn) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[vsock] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[vsock] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vsock] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}

}
