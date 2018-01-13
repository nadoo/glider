// http proxy
// NOTE: never keep-alive so the implementation can be much easier.

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// HTTP struct
type HTTP struct {
	*Forwarder        // as client
	sDialer    Dialer // dialer for server

	user     string
	password string
	xff      bool

	selfip string
}

// NewHTTP returns a http proxy.
func NewHTTP(addr, user, pass, rawQuery string, cDialer Dialer, sDialer Dialer) (*HTTP, error) {
	s := &HTTP{
		Forwarder: NewForwarder(addr, cDialer),
		sDialer:   sDialer,
		user:      user,
		password:  pass,
		xff:       false,
		selfip:    OutboundIP(),
	}

	p, _ := url.ParseQuery(rawQuery)
	if v, ok := p["xff"]; ok {
		if v[0] == "true" {
			s.xff = true
		}
	}

	return s, nil
}

// ListenAndServe .
func (s *HTTP) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	logf("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve .
func (s *HTTP) Serve(c net.Conn) {
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
		logf("read header error:%s", err)
		return
	}
	cleanHeaders(reqHeader)
	// tell the remote server not to keep alive
	reqHeader.Set("Connection", "close")

	// X-Forwarded-For
	if s.xff {
		if reqHeader.Get("") != "" {
			reqHeader.Add("X-Forwarded-For", ",")
		}
		reqHeader.Add("X-Forwarded-For", c.RemoteAddr().(*net.TCPAddr).IP.String())
		reqHeader.Add("X-Forwarded-For", ",")
		reqHeader.Add("X-Forwarded-For", s.selfip)
	}

	url, err := url.ParseRequestURI(requestURI)
	if err != nil {
		logf("parse request url error: %s", err)
		return
	}

	var tgt = url.Host
	if !strings.Contains(url.Host, ":") {
		tgt += ":80"
	}

	rc, err := s.sDialer.Dial("tcp", tgt)
	if err != nil {
		fmt.Fprintf(c, "%s 502 ERROR\r\n\r\n", proto)
		logf("failed to dial: %v", err)
		return
	}
	defer rc.Close()

	// GET http://example.com/a/index.htm HTTP/1.1 -->
	// GET /a/index.htm HTTP/1.1
	url.Scheme = ""
	url.Host = ""
	uri := url.String()

	var reqBuf bytes.Buffer
	writeFirstLine(method, uri, proto, &reqBuf)
	writeHeaders(reqHeader, &reqBuf)

	// send request to remote server
	rc.Write(reqBuf.Bytes())

	// copy the left request bytes to remote server. eg. length specificed or chunked body.
	go func() {
		io.Copy(rc, reqR)
		rc.SetDeadline(time.Now())
		c.SetDeadline(time.Now())
	}()

	respR := bufio.NewReader(rc)
	respTP := textproto.NewReader(respR)
	proto, code, status, ok := parseFirstLine(respTP)
	if !ok {
		return
	}

	respHeader, err := respTP.ReadMIMEHeader()
	if err != nil {
		logf("read header error:%s", err)
		return
	}

	respHeader.Set("Proxy-Connection", "close")
	respHeader.Set("Connection", "close")

	var respBuf bytes.Buffer
	writeFirstLine(proto, code, status, &respBuf)
	writeHeaders(respHeader, &respBuf)

	logf("proxy-http %s <-> %s", c.RemoteAddr(), tgt)
	c.Write(respBuf.Bytes())

	io.Copy(c, respR)

}

func (s *HTTP) servHTTPS(method, requestURI, proto string, c net.Conn) {
	rc, err := s.sDialer.Dial("tcp", requestURI)
	if err != nil {
		c.Write([]byte(proto))
		c.Write([]byte(" 502 ERROR\r\n\r\n"))
		logf("failed to dial: %v", err)
		return
	}

	c.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))

	logf("proxy-https %s <-> %s", c.RemoteAddr(), requestURI)

	_, _, err = relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		logf("relay error: %v", err)
	}
}

// Dial connects to the address addr on the network net via the proxy.
func (s *HTTP) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.cDialer.Dial("tcp", s.addr)
	if err != nil {
		logf("dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := rc.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	rc.Write([]byte("CONNECT " + addr + " HTTP/1.0\r\n"))
	rc.Write([]byte("Proxy-Connection: close\r\n"))

	if s.user != "" && s.password != "" {
		auth := s.user + ":" + s.password
		rc.Write([]byte("Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(auth)) + "\r\n"))
	}

	//header ended
	rc.Write([]byte("\r\n"))

	respR := bufio.NewReader(rc)
	respTP := textproto.NewReader(respR)
	_, code, _, ok := parseFirstLine(respTP)
	if ok && code == "200" {
		return rc, err
	} else if code == "407" {
		logf("proxy-http authencation needed by proxy %s", s.addr)
	} else if code == "405" {
		logf("proxy-http 'CONNECT' method not allowed by proxy %s", s.addr)
	}

	return nil, errors.New("proxy-http cound not connect remote address: " + addr + ". error code: " + code)
}

// DialUDP .
func (s *HTTP) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, errors.New("proxy-http udp not supported by http proxy now")
}

// parseFirstLine parses "GET /foo HTTP/1.1" OR "HTTP/1.1 200 OK" into its three parts.
func parseFirstLine(tp *textproto.Reader) (r1, r2, r3 string, ok bool) {
	line, err := tp.ReadLine()
	// logf("first line: %s", line)
	if err != nil {
		logf("read request line error:%s", err)
		return
	}

	s1 := strings.Index(line, " ")
	s2 := strings.Index(line[s1+1:], " ")
	if s1 < 0 || s2 < 0 {
		return
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func cleanHeaders(header textproto.MIMEHeader) {
	header.Del("Proxy-Connection")
	header.Del("Connection")
	header.Del("Keep-Alive")
	header.Del("Proxy-Authenticate")
	header.Del("Proxy-Authorization")
	header.Del("TE")
	header.Del("Trailers")
	header.Del("Transfer-Encoding")
	header.Del("Upgrade")
}

func writeFirstLine(s1, s2, s3 string, buf *bytes.Buffer) {
	buf.Write([]byte(s1))
	buf.Write([]byte(" "))
	buf.Write([]byte(s2))
	buf.Write([]byte(" "))
	buf.Write([]byte(s3))
	buf.Write([]byte("\r\n"))
}

func writeHeaders(header textproto.MIMEHeader, buf *bytes.Buffer) {
	for key, values := range header {
		buf.Write([]byte(key))
		buf.Write([]byte(": "))
		for k, v := range values {
			buf.Write([]byte(v))
			if k > 0 {
				buf.Write([]byte(" "))
			}
		}
		buf.Write([]byte("\r\n"))
	}

	//header ended
	buf.Write([]byte("\r\n"))
}
