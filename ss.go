package main

import (
	"errors"
	"log"
	"net"
	"strings"

	"github.com/shadowsocks/go-shadowsocks2/core"
)

const udpBufSize = 65536

// SS .
type SS struct {
	*Forwarder
	sDialer Dialer

	core.Cipher
}

// NewSS returns a shadowsocks proxy.
func NewSS(addr, method, pass string, cDialer Dialer, sDialer Dialer) (*SS, error) {
	ciph, err := core.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("PickCipher for '%s', error: %s", method, err)
	}

	s := &SS{
		Forwarder: NewForwarder(addr, cDialer),
		sDialer:   sDialer,
		Cipher:    ciph,
	}

	return s, nil
}

// ListenAndServe serves ss requests.
func (s *SS) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("proxy-ss failed to listen on %s: %v", s.addr, err)
		return
	}

	logf("proxy-ss listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("proxy-ss failed to accept: %v", err)
			continue
		}
		go s.Serve(c)
	}
}

// Serve .
func (s *SS) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConn(c)

	tgt, err := ReadAddr(c)
	if err != nil {
		logf("proxy-ss failed to get target address: %v", err)
		return
	}

	dialer := s.sDialer.NextDialer("")

	// udp over tcp?
	uot := UoT(tgt[0])
	if uot && dialer.Addr() == "DIRECT" {

		rc, err := net.ListenPacket("udp", "")
		if err != nil {
			logf("UDP remote listen error: %v", err)
		}
		defer rc.Close()

		req := make([]byte, udpBufSize)
		n, err := c.Read(req)
		if err != nil {
			logf("error in ioutil.ReadAll: %s\n", err)
			return
		}

		tgtAddr, _ := net.ResolveUDPAddr("udp", tgt.String())
		rc.WriteTo(req[:n], tgtAddr)

		buf := make([]byte, udpBufSize)
		n, _, err = rc.ReadFrom(buf)
		if err != nil {
			logf("proxy-uottun read error: %v", err)
		}

		c.Write(buf[:n])

		logf("proxy-ss %s <-tcp-> %s - %s <-udp-> %s ", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

		return
	}

	network := "tcp"
	if uot {
		network = "udp"
	}

	rc, err := dialer.Dial(network, tgt.String())
	if err != nil {
		logf("proxy-ss failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	logf("proxy-ss %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		logf("relay error: %v", err)
	}

}

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {

	target := ParseAddr(addr)
	if target == nil {
		return nil, errors.New("Unable to parse address: " + addr)
	}

	// udp over tcp tag
	if network == "udp" {
		target[0] = target[0] | 0x8
	}

	c, err := s.cDialer.Dial("tcp", s.addr)
	if err != nil {
		logf("dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConn(c)
	if _, err = c.Write(target); err != nil {
		c.Close()
		return nil, err
	}

	return c, err
}

// ListCipher .
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}
