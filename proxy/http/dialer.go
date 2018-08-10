package http

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"net"
	"net/textproto"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("http", CreateDialer)
}

// Dialer struct
type Dialer struct {
	*HTTP
	dialer proxy.Dialer
}

// NewDialer returns a proxy dialer
func NewDialer(s string, dialer proxy.Dialer) (*Dialer, error) {
	h, err := NewHTTP(s)
	if err != nil {
		return nil, err
	}

	d := &Dialer{HTTP: h, dialer: dialer}
	return d, nil
}

// CreateDialer returns a proxy dialer
func CreateDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewDialer(s, dialer)
}

// Addr returns dialer's address
func (s *Dialer) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// NextDialer returns the next dialer
func (s *Dialer) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial establishes a connection to the addr
func (s *Dialer) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[http] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	var buf bytes.Buffer
	buf.Write([]byte("CONNECT " + addr + " HTTP/1.1\r\n"))
	buf.Write([]byte("Proxy-Connection: Keep-Alive\r\n"))

	if s.user != "" && s.password != "" {
		auth := s.user + ":" + s.password
		buf.Write([]byte("Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(auth)) + "\r\n"))
	}

	//header ended
	buf.Write([]byte("\r\n"))
	rc.Write(buf.Bytes())

	respR := bufio.NewReader(rc)
	respTP := textproto.NewReader(respR)
	_, code, _, ok := parseFirstLine(respTP)
	if ok && code == "200" {
		return rc, err
	} else if code == "407" {
		log.F("[http] authencation needed by proxy %s", s.addr)
	} else if code == "405" {
		log.F("[http] 'CONNECT' method not allowed by proxy %s", s.addr)
	}

	return nil, errors.New("[http] can not connect remote address: " + addr + ". error code: " + code)
}

// DialUDP returns a PacketConn to the addr
func (s *Dialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return nil, nil, errors.New("http client does not support udp")
}
