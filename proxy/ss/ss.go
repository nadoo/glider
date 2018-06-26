package ss

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shadowsocks/go-shadowsocks2/core"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

const udpBufSize = 65536

// SS .
type SS struct {
	dialer proxy.Dialer
	addr   string

	core.Cipher
}

func init() {
	proxy.RegisterDialer("ss", NewSSDialer)
	proxy.RegisterServer("ss", NewSSServer)
}

// NewSS returns a shadowsocks proxy.
func NewSS(s string, dialer proxy.Dialer) (*SS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	var method, pass string
	if u.User != nil {
		method = u.User.Username()
		pass, _ = u.User.Password()
	}

	ciph, err := core.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("proxy-ss PickCipher for '%s', error: %s", method, err)
	}

	p := &SS{
		dialer: dialer,
		addr:   addr,
		Cipher: ciph,
	}

	return p, nil
}

// NewSSDialer returns a ss proxy dialer.
func NewSSDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewSS(s, dialer)
}

// NewSSServer returns a ss proxy server.
func NewSSServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewSS(s, dialer)
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
		log.F("proxy-ss failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("proxy-ss listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("proxy-ss failed to accept: %v", err)
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

	tgt, err := socks.ReadAddr(c)
	if err != nil {
		log.F("proxy-ss failed to get target address: %v", err)
		return
	}

	dialer := s.dialer.NextDialer(tgt.String())

	// udp over tcp?
	uot := socks.UoT(tgt[0])
	if uot && dialer.Addr() == "DIRECT" {

		rc, err := net.ListenPacket("udp", "")
		if err != nil {
			log.F("proxy-ss UDP remote listen error: %v", err)
		}
		defer rc.Close()

		req := make([]byte, udpBufSize)
		n, err := c.Read(req)
		if err != nil {
			log.F("proxy-ss error in ioutil.ReadAll: %s\n", err)
			return
		}

		tgtAddr, _ := net.ResolveUDPAddr("udp", tgt.String())
		rc.WriteTo(req[:n], tgtAddr)

		buf := make([]byte, udpBufSize)
		n, _, err = rc.ReadFrom(buf)
		if err != nil {
			log.F("proxy-uottun read error: %v", err)
		}

		c.Write(buf[:n])

		log.F("proxy-ss %s <-tcp-> %s - %s <-udp-> %s ", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

		return
	}

	network := "tcp"
	if uot {
		network = "udp"
	}

	rc, err := dialer.Dial(network, tgt.String())
	if err != nil {
		log.F("proxy-ss failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	log.F("proxy-ss %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("proxy-ss relay error: %v", err)
	}

}

// ListenAndServeUDP serves udp ss requests.
func (s *SS) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("proxy-ss-udp failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	lc = s.PacketConn(lc)

	log.F("proxy-ss-udp listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, udpBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("proxy-ss-udp remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			lpc, nextHop, err := s.dialer.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("proxy-ss-udp remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false)
			nm.Store(raddr.String(), pc)

			go func() {
				conn.TimedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(*PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("proxy-ss-udp remote write error: %v", err)
			continue
		}

		log.F("proxy-ss-udp %s <-> %s", raddr, c.tgtAddr)
	}
}

// ListCipher .
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}

// Addr returns forwarder's address
func (s *SS) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *SS) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, errors.New("proxy-ss unable to parse address: " + addr)
	}

	if network == "uot" {
		target[0] = target[0] | 0x8
	}

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("proxy-ss dial to %s error: %s", s.addr, err)
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
	pc, nextHop, err := s.dialer.DialUDP(network, s.addr)
	if err != nil {
		log.F("proxy-ss dialudp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), nextHop, socks.ParseAddr(addr), true)
	return pkc, nextHop, err
}

// PktConn .
type PktConn struct {
	net.PacketConn

	writeAddr net.Addr // write to and read from addr

	tgtAddr   socks.Addr
	tgtHeader bool
}

// NewPktConn returns a PktConn
func NewPktConn(c net.PacketConn, writeAddr net.Addr, tgtAddr socks.Addr, tgtHeader bool) *PktConn {
	pc := &PktConn{
		PacketConn: c,
		writeAddr:  writeAddr,
		tgtAddr:    tgtAddr,
		tgtHeader:  tgtHeader}
	return pc
}

// ReadFrom overrides the original function from net.PacketConn
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.ReadFrom(b)
	}

	buf := make([]byte, len(b))
	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, err
	}

	tgtAddr := socks.SplitAddr(buf)
	copy(b, buf[len(tgtAddr):])

	//test
	if pc.writeAddr == nil {
		pc.writeAddr = raddr
	}

	if pc.tgtAddr == nil {
		pc.tgtAddr = tgtAddr
	}

	return n - len(tgtAddr), raddr, err
}

// WriteTo overrides the original function from net.PacketConn
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.WriteTo(b, addr)
	}

	buf := make([]byte, len(pc.tgtAddr)+len(b))
	copy(buf, pc.tgtAddr)
	copy(buf[len(pc.tgtAddr):], b)

	return pc.PacketConn.WriteTo(buf, pc.writeAddr)
}
