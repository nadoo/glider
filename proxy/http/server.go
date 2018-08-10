package http

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("http", CreateServer)
}

// Server struct
type Server struct {
	*HTTP
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewHTTP(s)
	if err != nil {
		return nil, err
	}
	server := &Server{HTTP: h, Forwarder: f}
	return server, nil
}

// CreateServer returns a local proxy server
func CreateServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewServer(s, f)
}

// ListenAndServe serves requests from clients
func (s *Server) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[http] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve .
func (s *Server) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	reqR := bufio.NewReader(c)
	reqTP := textproto.NewReader(reqR)
	method, requestURI, proto, ok := parseFirstLine(reqTP)
	if !ok {
		return
	}

	if method == "CONNECT" {
		s.servHTTPS(method, requestURI, proto, c)
		return
	}

	reqHeader, err := reqTP.ReadMIMEHeader()
	if err != nil {
		log.F("read header error:%s", err)
		return
	}
	cleanHeaders(reqHeader)

	// tell the remote server not to keep alive
	reqHeader.Set("Connection", "close")

	u, err := url.ParseRequestURI(requestURI)
	if err != nil {
		log.F("[http] parse request url error: %s", err)
		return
	}

	var tgt = u.Host
	if !strings.Contains(u.Host, ":") {
		tgt += ":80"
	}

	rc, err := s.Dial("tcp", tgt)
	if err != nil {
		fmt.Fprintf(c, "%s 502 ERROR\r\n\r\n", proto)
		log.F("[http] failed to dial: %v", err)
		return
	}
	defer rc.Close()

	// GET http://example.com/a/index.htm HTTP/1.1 -->
	// GET /a/index.htm HTTP/1.1
	u.Scheme = ""
	u.Host = ""
	uri := u.String()

	var reqBuf bytes.Buffer
	writeFirstLine(method, uri, proto, &reqBuf)
	writeHeaders(reqHeader, &reqBuf)

	// send request to remote server
	rc.Write(reqBuf.Bytes())

	// copy the left request bytes to remote server. eg. length specificed or chunked body.
	go func() {
		if _, err := reqR.Peek(1); err == nil {
			io.Copy(rc, reqR)
			rc.SetDeadline(time.Now())
			c.SetDeadline(time.Now())
		}
	}()

	respR := bufio.NewReader(rc)
	respTP := textproto.NewReader(respR)
	proto, code, status, ok := parseFirstLine(respTP)
	if !ok {
		return
	}

	respHeader, err := respTP.ReadMIMEHeader()
	if err != nil {
		log.F("[http] read header error:%s", err)
		return
	}

	respHeader.Set("Proxy-Connection", "close")
	respHeader.Set("Connection", "close")

	var respBuf bytes.Buffer
	writeFirstLine(proto, code, status, &respBuf)
	writeHeaders(respHeader, &respBuf)

	log.F("[http] %s <-> %s", c.RemoteAddr(), tgt)
	c.Write(respBuf.Bytes())

	io.Copy(c, respR)

}

func (s *Server) servHTTPS(method, requestURI, proto string, c net.Conn) {
	rc, err := s.Dial("tcp", requestURI)
	if err != nil {
		c.Write([]byte(proto))
		c.Write([]byte(" 502 ERROR\r\n\r\n"))
		log.F("[http] failed to dial: %v", err)
		return
	}

	c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

	log.F("[http] %s <-> %s [c]", c.RemoteAddr(), requestURI)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("relay error: %v", err)
	}
}
