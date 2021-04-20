package smux

import (
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"

	"github.com/nadoo/glider/proxy/protocol/smux"
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
	transport := strings.Split(s, ",")

	u, err := url.Parse(transport[0])
	if err != nil {
		log.F("[smux] parse url err: %s", err)
		return nil, err
	}

	m := &SmuxServer{
		proxy: p,
		addr:  u.Host,
	}

	if len(transport) > 1 {
		m.server, err = proxy.ServerFromURL(transport[1], p)
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
		log.F("[smux] failed to listen on %s: %v", s.addr, err)
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

func (s *SmuxServer) ServeStream(c *smux.Stream) {
	if s.server != nil {
		s.server.Serve(c)
	}
}
