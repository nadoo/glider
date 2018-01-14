package main

import (
	"errors"
	"log"
	"net"
	"strings"
	"sync"
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
// TODO: Forwarder chain not supported now.
func (s *SS) ListenAndServeUDP() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-ss-udp failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-ss-udp listening UDP on %s", s.addr)

	c = s.PacketConn(c)

	var nm sync.Map
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

		tgtUDPAddr, err := net.ResolveUDPAddr("udp", tgtAddr.String())
		if err != nil {
			logf("proxy-ss-udp failed to resolve target UDP address: %v", err)
			continue
		}

		logf("proxy-ss-udp %s <-> %s", raddr, tgtAddr)

		payload := buf[len(tgtAddr):n]

		var pc net.PacketConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				logf("proxy-ss-udp remote listen error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)
			go func() {
				timedCopy(c, raddr, pc, 5*time.Minute, true)
				pc.Close()
				nm.Delete(raddr.String())
			}()
		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(payload, tgtUDPAddr) // accept only UDPAddr despite the signature
		if err != nil {
			logf("proxy-ss-udp remote write error: %v", err)
			continue
		}

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

	switch network {
	case "tcp":
		return s.dialTCP(target)
	case "uot":
		target[0] = target[0] | 0x8
		return s.dialTCP(target)
	case "udp":
		return s.dialUDP(target)
	default:
		return nil, errors.New("Unknown schema: " + network)
	}

}

// DialTCP connects to the address addr via the proxy.
func (s *SS) dialTCP(target Addr) (net.Conn, error) {
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

// TODO: support forwarder chain
func (s *SS) dialUDP(target Addr) (net.Conn, error) {
	c, err := net.ListenPacket("udp", "")
	if err != nil {
		logf("proxy-ss dialudp failed to listen packet: %v", err)
		return nil, err
	}

	sUDPAddr, err := net.ResolveUDPAddr("udp", s.Addr())
	suc := &ssUDPConn{
		PacketConn: s.PacketConn(c),
		addr:       sUDPAddr,
		target:     target,
	}

	return suc, err
}

type ssUDPConn struct {
	net.PacketConn

	addr   net.Addr
	target Addr
}

func (uc *ssUDPConn) Read(b []byte) (int, error) {
	buf := make([]byte, len(b))
	n, raddr, err := uc.PacketConn.ReadFrom(buf)
	if err != nil {
		return 0, err
	}

	srcAddr := ParseAddr(raddr.String())
	copy(b, buf[len(srcAddr):])

	return n - len(srcAddr), err
}

func (uc *ssUDPConn) Write(b []byte) (int, error) {
	buf := make([]byte, len(uc.target)+len(b))
	copy(buf, uc.target)
	copy(buf[len(uc.target):], b)

	// logf("Write: \n%s", hex.Dump(buf))

	return uc.PacketConn.WriteTo(buf, uc.addr)
}

func (uc *ssUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return 0, errors.New("not available")
}

func (uc *ssUDPConn) RemoteAddr() net.Addr {
	return uc.addr
}
