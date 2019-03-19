package ss

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/go-shadowsocks2/core"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

// SS is a base ss struct.
type SS struct {
	dialer proxy.Dialer
	addr   string

	core.Cipher
}

func init() {
	proxy.RegisterDialer("ss", NewSSDialer)
	proxy.RegisterServer("ss", NewSSServer)
}

// NewSS returns a ss proxy.
func NewSS(s string, dialer proxy.Dialer) (*SS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	method := u.User.Username()
	pass, _ := u.User.Password()

	ciph, err := core.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("[ss] PickCipher for '%s', error: %s", method, err)
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
		log.F("[ss] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[ss] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[ss] failed to accept: %v", err)
			continue
		}
		go s.Serve(c)
	}

}

// Serve serves a connection.
func (s *SS) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConn(c)

	tgt, err := socks.ReadAddr(c)
	if err != nil {
		log.F("[ss] failed to get target address: %v", err)
		return
	}

	dialer := s.dialer.NextDialer(tgt.String())

	// udp over tcp?
	uot := socks.UoT(tgt[0])
	if uot && dialer.Addr() == "DIRECT" {
		rc, err := net.ListenPacket("udp", "")
		if err != nil {
			log.F("[ss-uottun] UDP remote listen error: %v", err)
		}
		defer rc.Close()

		req := make([]byte, conn.UDPBufSize)
		n, err := c.Read(req)
		if err != nil {
			log.F("[ss-uottun] error in ioutil.ReadAll: %s\n", err)
			return
		}

		tgtAddr, _ := net.ResolveUDPAddr("udp", tgt.String())
		rc.WriteTo(req[:n], tgtAddr)

		buf := make([]byte, conn.UDPBufSize)
		n, _, err = rc.ReadFrom(buf)
		if err != nil {
			log.F("[ss-uottun] read error: %v", err)
		}

		c.Write(buf[:n])

		log.F("[ss] %s <-tcp-> %s - %s <-udp-> %s ", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

		return
	}

	network := "tcp"
	if uot {
		network = "udp"
	}

	rc, err := dialer.Dial(network, tgt.String())
	if err != nil {
		log.F("[ss] %s <-> %s, error in dial: %v", c.RemoteAddr(), tgt, err)
		return
	}
	defer rc.Close()

	log.F("[ss] %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("[ss] relay error: %v", err)
	}

}

// ListenAndServeUDP serves udp ss requests.
func (s *SS) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[ss-udp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	lc = s.PacketConn(lc)

	log.F("[ss-udp] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[ss-udp] remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			lpc, nextHop, err := s.dialer.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("[ss-udp] remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false)
			nm.Store(raddr.String(), pc)

			go func() {
				conn.RelayUDP(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

			log.F("[ss-udp] %s <-> %s", raddr, c.tgtAddr)

		} else {
			pc = v.(*PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("[ss-udp] remote write error: %v", err)
			continue
		}

		// log.F("[ss-udp] %s <-> %s", raddr, c.tgtAddr)
	}
}

// ListCipher returns all the ciphers supported.
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}

// Addr returns forwarder's address.
func (s *SS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// NextDialer returns the next dialer.
func (s *SS) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, errors.New("[ss] unable to parse address: " + addr)
	}

	if network == "uot" {
		target[0] = target[0] | 0x8
	}

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[ss] dial to %s error: %s", s.addr, err)
		return nil, err
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
		log.F("[ss] dialudp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), nextHop, socks.ParseAddr(addr), true)
	return pkc, nextHop, err
}
