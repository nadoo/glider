package pxyproto

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("pxyproto", NewPxyProtoServer)
}

// PxyProtoServer struct.
type PxyProtoServer struct {
	addr   string
	proxy  proxy.Proxy
	server proxy.Server
}

// NewPxyProtoServer returns a PxyProtoServer struct.
func NewPxyProtoServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	u, err := url.Parse(schemes[0])
	if err != nil {
		log.F("[pxyproto] parse url err: %s", err)
		return nil, err
	}

	t := &PxyProtoServer{proxy: p, addr: u.Host}
	if len(schemes) < 2 {
		return nil, errors.New("[pxyproto] you must use pxyproto with a proxy server, e.g: pxyproto://:1234,http://")
	}

	t.server, err = proxy.ServerFromURL(schemes[1], p)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// ListenAndServe listens on server's addr and serves connections.
func (s *PxyProtoServer) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[pxyproto] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[pxyproto] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[pxyproto] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *PxyProtoServer) Serve(cc net.Conn) {
	c, err := newServerConn(cc)
	if err != nil {
		log.F("[pxyproto] parse header failed, error: %v", err)
		cc.Close()
		return
	}

	// log.F("[pxyproto] %s <-> %s <-> %s <-> %s",
	// c.RemoteAddr(), c.LocalAddr(), cc.RemoteAddr(), cc.LocalAddr())

	if s.server != nil {
		s.server.Serve(c)
		return
	}
}

type serverConn struct {
	*proxy.Conn
	src, dst net.Addr
}

func newServerConn(c net.Conn) (*serverConn, error) {
	sc := &serverConn{
		Conn: proxy.NewConn(c),
		src:  c.RemoteAddr(),
		dst:  c.LocalAddr(),
	}
	return sc, sc.parseHeader()
}

// "PROXY TCPx SRC_IP DST_IP SRC_PORT DST_PORT"
func (c *serverConn) parseHeader() error {
	line, err := c.Conn.Reader().ReadString('\n')
	if err != nil {
		return err
	}

	line = strings.ReplaceAll(line, "\r\n", "")
	// log.F("[pxyproto] req header: %s", line)

	header := strings.Split(line, " ")
	if len(header) != 6 {
		return fmt.Errorf("invalid header: %s", line)
	}

	if header[0] != "PROXY" {
		return fmt.Errorf("invalid header: %s", line)
	}

	c.src, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(header[2], header[4]))
	if err != nil {
		return fmt.Errorf("parse header: %s, error: %v", line, err)
	}

	c.dst, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(header[3], header[5]))
	if err != nil {
		return fmt.Errorf("parse header: %s, error: %v", line, err)
	}

	return nil
}

func (c *serverConn) LocalAddr() net.Addr  { return c.dst }
func (c *serverConn) RemoteAddr() net.Addr { return c.src }
