package main

import (
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"
)

const udpBufSize = 64 * 1024

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
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP serves tcp ss requests.
func (s *SS) ListenAndServeTCP() {
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
		go s.ServeTCP(c)
	}
}

// ServeTCP .
func (s *SS) ServeTCP(c net.Conn) {
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

	rc, err := s.sDialer.Dial("tcp", tgt.String())
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

// ListenAndServeUDP serves udp ss requests.
func (s *SS) ListenAndServeUDP() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-ss failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-ss listening UDP on %s", s.addr)

	c = s.PacketConn(c)

	var nm sync.Map
	buf := make([]byte, udpBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			logf("UDP remote read error: %v", err)
			continue
		}

		tgtAddr := socks.SplitAddr(buf[:n])
		if tgtAddr == nil {
			logf("failed to split target address from packet: %q", buf[:n])
			continue
		}

		tgtUDPAddr, err := net.ResolveUDPAddr("udp", tgtAddr.String())
		if err != nil {
			logf("failed to resolve target UDP address: %v", err)
			continue
		}

		payload := buf[len(tgtAddr):n]

		var pc net.PacketConn
		v, _ := nm.Load(raddr.String())
		if v == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				logf("UDP remote listen error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)
			go func() {
				timedCopy(c, raddr, pc, 5*time.Minute, true)
				pc.Close()
				nm.Delete(raddr.String())
			}()
		}

		pc = pc.(net.PacketConn)
		_, err = pc.WriteTo(payload, tgtUDPAddr) // accept only UDPAddr despite the signature
		if err != nil {
			logf("UDP remote write error: %v", err)
			continue
		}

	}
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {

	target := ParseAddr(addr)
	if target == nil {
		return nil, errors.New("Unable to parse address: " + addr)
	}

	c, err := s.cDialer.Dial(network, s.addr)
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
