package smux

import (
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/smux"
	"github.com/nadoo/glider/proxy"
)

// SmuxServer struct.
type SmuxServer struct {
	proxy  proxy.Proxy
	addr   string
	server proxy.Server
}

func init() {
	proxy.RegisterServer("smux", NewSmuxServer)
}

// NewSmuxServer returns a smux transport layer before the real server.
func NewSmuxServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	u, err := url.Parse(schemes[0])
	if err != nil {
		log.F("[smux] parse url err: %s", err)
		return nil, err
	}

	m := &SmuxServer{
		proxy: p,
		addr:  u.Host,
	}

	if len(schemes) > 1 {
		m.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// ListenAndServe listens on server's addr and serves connections.
func (s *SmuxServer) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[smux] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[smux] listening mux on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[smux] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *SmuxServer) Serve(c net.Conn) {
	// we know the internal server will close the connection after serve
	// defer c.Close()

	session, err := smux.Server(c, nil)
	if err != nil {
		log.F("[smux] failed to create session: %v", err)
		return
	}

	for {
		// Accept a stream
		stream, err := session.AcceptStream()
		if err != nil {
			session.Close()
			break
		}
		go s.ServeStream(stream)
	}
}

// ServeStream serves a smux stream.
func (s *SmuxServer) ServeStream(c *smux.Stream) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[smux] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[smux] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[smux] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}

}
