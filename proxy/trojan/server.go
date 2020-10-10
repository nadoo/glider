package trojan

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

// NewTrojanServer returns a trojan proxy server.
func NewTrojanServer(s string, p proxy.Proxy) (proxy.Server, error) {
	t, err := NewTrojan(s, nil, p)
	if err != nil {
		log.F("[trojan] create instance error: %s", err)
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(t.certFile, t.keyFile)
	if err != nil {
		log.F("[trojan] unable to load cert: %s, key %s", t.certFile, t.keyFile)
		return nil, err
	}

	t.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return t, err
}

// ListenAndServe listen and serves connections.
func (s *Trojan) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[trojan] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[trojan] listening TCP on %s with TLS", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[trojan] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *Trojan) Serve(cc net.Conn) {
	defer cc.Close()

	if cc, ok := cc.(*net.TCPConn); ok {
		cc.SetKeepAlive(true)
	}

	c := tls.Server(cc, s.tlsConfig)
	err := c.Handshake()
	if err != nil {
		log.F("[trojan] error in tls handshake: %s", err)
		return
	}

	cmd, target, err := s.readHeader(c)
	if err != nil {
		log.F("[trojan] error in server handshake: %s", err)
		return
	}

	network := "tcp"
	dialer := s.proxy.NextDialer(target.String())

	if cmd == socks.CmdUDPAssociate {
		// there is no upstream proxy, just serve it
		if dialer.Addr() == "DIRECT" {
			s.ServeUoT(c, target)
			return
		}
		network = "udp"
	}

	rc, err := dialer.Dial(network, target.String())
	if err != nil {
		log.F("[trojan] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[trojan] %s <-> %s via %s", c.RemoteAddr(), target, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[trojan] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

func (s *Trojan) readHeader(c net.Conn) (byte, socks.Addr, error) {
	// pass: 56, "\r\n": 2, cmd: 1
	buf := pool.GetBuffer(59)
	defer pool.PutBuffer(buf)

	if _, err := io.ReadFull(c, buf); err != nil {
		return socks.CmdError, nil, err
	}

	// pass, 56bytes
	if !bytes.Equal(buf[:56], s.pass[:]) {
		return socks.CmdError, nil, errors.New("wrong password")
	}

	// cmd, 1byte
	cmd := byte(buf[58])

	// target
	tgt, err := socks.ReadAddr(c)
	if err != nil {
		return cmd, nil, fmt.Errorf("read target address error: %v", err)
	}

	// "\r\n", 2bytes
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return socks.CmdError, tgt, err
	}

	return cmd, tgt, nil
}

// ServeUoT serves udp over tcp requests.
func (s *Trojan) ServeUoT(c net.Conn, tgt socks.Addr) {
	rc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.F("[trojan] UDP remote listen error: %v", err)
		return
	}
	defer rc.Close()

	tgtAddr, err := net.ResolveUDPAddr("udp", tgt.String())
	if err != nil {
		log.F("[vless] error in ResolveUDPAddr: %v", err)
		return
	}

	pc := NewPktConn(c, tgt)

	go func() {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		defer pool.PutBuffer(buf)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				log.F("[trojan] read error: %s\n", err)
				return
			}

			_, err = rc.WriteTo(buf[:n], tgtAddr)
			if err != nil {
				log.F("[trojan] write rc error: %s\n", err)
				return
			}
		}
	}()

	log.F("[trojan] %s <-tcp-> %s - %s <-udp-> %s", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

	buf := pool.GetBuffer(proxy.UDPBufSize)
	defer pool.PutBuffer(buf)

	for {
		n, _, err := rc.ReadFrom(buf)
		if err != nil {
			log.F("[trojan] read rc error: %v", err)
			break
		}

		_, err = pc.WriteTo(buf[:n], nil)
		if err != nil {
			log.F("[trojan] write pc error: %v", err)
			break
		}
	}
}
