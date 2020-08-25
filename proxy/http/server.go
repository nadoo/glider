package http

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/pool"
	"github.com/nadoo/glider/proxy"
)

// NewHTTPServer returns a http proxy server.
func NewHTTPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewHTTP(s, nil, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (s *HTTP) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[http] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[http] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[http] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *HTTP) Serve(cc net.Conn) {
	defer cc.Close()

	var c *conn.Conn
	switch cc := cc.(type) {
	case *conn.Conn:
		c = cc
	case *net.TCPConn:
		cc.SetKeepAlive(true)
		c = conn.NewConn(cc)
	default:
		c = conn.NewConn(cc)
	}

	req, err := parseRequest(c.Reader())
	if err != nil {
		log.F("[http] can not parse request from %s", c.RemoteAddr())
		return
	}

	if s.pretend {
		fmt.Fprintf(c, "%s 404 Not Found\r\nServer: nginx\r\n\r\n404 Not Found\r\n", req.proto)
		log.F("[http] %s <-> %s,pretend as web server", c.RemoteAddr().String(), s.Addr())
		return
	}

	s.servRequest(req, c)
}

func (s *HTTP) servRequest(req *request, c *conn.Conn) {
	// Auth
	if s.user != "" && s.password != "" {
		if user, pass, ok := extractUserPass(req.auth); !ok || user != s.user || pass != s.password {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic\r\n\r\n")
			log.F("[http] auth failed from %s, auth info: %s:%s", c.RemoteAddr(), user, pass)
			return
		}
	}

	if req.method == "CONNECT" {
		s.servHTTPS(req, c)
		return
	}

	s.servHTTP(req, c)
}

func (s *HTTP) servHTTPS(r *request, c net.Conn) {
	rc, dialer, err := s.proxy.Dial("tcp", r.uri)
	if err != nil {
		io.WriteString(c, r.proto+" 502 ERROR\r\n\r\n")
		log.F("[http] %s <-> %s [c] via %s, error in dial: %v", c.RemoteAddr(), r.uri, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")

	log.F("[http] %s <-> %s [c] via %s", c.RemoteAddr(), r.uri, dialer.Addr())

	if err = conn.Relay(c, rc); err != nil {
		log.F("[http] relay error: %v", err)
		s.proxy.Record(dialer, false)
	}
}

func (s *HTTP) servHTTP(req *request, c *conn.Conn) {
	rc, dialer, err := s.proxy.Dial("tcp", req.target)
	if err != nil {
		fmt.Fprintf(c, "%s 502 ERROR\r\n\r\n", req.proto)
		log.F("[http] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), req.target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	buf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(buf)

	// send request to remote server
	req.WriteBuf(buf)
	_, err = rc.Write(buf.Bytes())
	if err != nil {
		return
	}

	// copy the left request bytes to remote server. eg. length specificed or chunked body.
	go func() {
		if _, err := c.Reader().Peek(1); err == nil {
			b := pool.GetBuffer(conn.TCPBufSize)
			io.CopyBuffer(rc, c, b)
			pool.PutBuffer(b)

			rc.SetDeadline(time.Now())
			c.SetDeadline(time.Now())
		}
	}()

	r := bufio.NewReader(rc)
	tpr := textproto.NewReader(r)
	line, err := tpr.ReadLine()
	if err != nil {
		return
	}

	proto, code, status, ok := parseStartLine(line)
	if !ok {
		return
	}

	header, err := tpr.ReadMIMEHeader()
	if err != nil {
		log.F("[http] read header error:%s", err)
		return
	}

	header.Set("Proxy-Connection", "close")
	header.Set("Connection", "close")

	buf.Reset()
	writeStartLine(buf, proto, code, status)
	writeHeaders(buf, header)

	log.F("[http] %s <-> %s", c.RemoteAddr(), req.target)
	c.Write(buf.Bytes())

	b := pool.GetBuffer(conn.TCPBufSize)
	io.CopyBuffer(c, r, b)
	pool.PutBuffer(b)
}
