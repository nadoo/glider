package main

import (
	"errors"
	"log"
	"net"
	"strings"
	"time"

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

// ServeTCP serves tcp ss requests.
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

	dialer := s.sDialer.NextDialer(tgt.String())

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
		logf("proxy-ss relay error: %v", err)
	}

}

// ListenAndServeUDP serves udp ss requests.
func (s *SS) ListenAndServeUDP() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-ss-udp failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-ss-udp listening UDP on %s", s.addr)

	c = s.PacketConn(c)
	buf := make([]byte, udpBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			logf("proxy-ss-udp remote read error: %v", err)
			continue
		}

		tgtAddr := SplitAddr(buf[:n])
		if tgtAddr == nil {
			logf("proxy-ss-udp failed to split target address from packet: %q", buf[:n])
			continue
		}

		logf("proxy-ss-udp %s <-> %s", raddr, tgtAddr)

		payload := buf[len(tgtAddr):n]

		rc, nexHop, err := s.sDialer.DialUDP("udp", tgtAddr.String())
		if err != nil {
			logf("proxy-ss-udp remote listen error: %v", err)
			continue
		}

		_, err = rc.WriteTo(payload, nexHop) // accept only UDPAddr despite the signature
		if err != nil {
			logf("proxy-ss-udp remote write error: %v", err)
			continue
		}

		rcBuf := make([]byte, udpBufSize)
		rc.SetReadDeadline(time.Now().Add(time.Minute))
		copy(rcBuf, tgtAddr)

		n, _, err = rc.ReadFrom(rcBuf[len(tgtAddr):])
		if err != nil {
			logf("proxy-ss-udp rc.Read error: %v", err)
			return
		}
		rc.Close()

		c.WriteTo(rcBuf[:len(tgtAddr)+n], raddr)
	}

}

// ListCipher .
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {
	target := ParseAddr(addr)
	if target == nil {
		return nil, errors.New("Unable to parse address: " + addr)
	}

	if network == "uot" {
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

// DialUDP connects to the given address via the proxy.
func (s *SS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	pc, nextHop, err := s.cDialer.DialUDP(network, s.addr)
	if err != nil {
		logf("proxy-ss dialudp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), nextHop, ParseAddr(addr), true)
	return pkc, nextHop, err
}

// PktConn wraps a net.PacketConn and support Write method like net.Conn
type PktConn struct {
	net.PacketConn

	addr      net.Addr // write to and read from addr
	target    Addr
	tgtHeader bool
}

// NewPktConn returns a PktConn
func NewPktConn(c net.PacketConn, addr net.Addr, target Addr, tgtHeader bool) *PktConn {
	pc := &PktConn{
		PacketConn: c,
		addr:       addr,
		target:     target,
		tgtHeader:  tgtHeader}
	return pc
}

func (pc *PktConn) Read(b []byte) (int, error) {
	n, _, err := pc.ReadFrom(b)
	return n, err
}

func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {

	if !pc.tgtHeader {
		return pc.PacketConn.ReadFrom(b)
	}

	buf := make([]byte, len(b))
	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, err
	}

	srcAddr := ParseAddr(raddr.String())
	copy(b, buf[len(srcAddr):])

	return n - len(srcAddr), raddr, err
}

func (pc *PktConn) Write(b []byte) (int, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.WriteTo(b, pc.addr)
	}

	buf := make([]byte, len(pc.target)+len(b))
	copy(buf, pc.target)
	copy(buf[len(pc.target):], b)

	return pc.PacketConn.WriteTo(buf, pc.addr)
}

func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.Write(b)
}

func (pc *PktConn) RemoteAddr() net.Addr {
	return pc.addr
}
