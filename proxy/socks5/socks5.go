// https://tools.ietf.org/html/rfc1928

// socks5 client:
// https://github.com/golang/net/tree/master/proxy
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// socks5 server:
// https://github.com/shadowsocks/go-shadowsocks2/tree/master/socks

package socks5

import (
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

// Version: socks5 version
const Version = 5

// SOCKS5 struct
type SOCKS5 struct {
	dialer   proxy.Dialer
	addr     string
	user     string
	password string
}

func init() {
	proxy.RegisterDialer("socks5", NewSocks5Dialer)
	proxy.RegisterServer("socks5", NewSocks5Server)
}

// NewSOCKS5 returns a Proxy that makes SOCKS v5 connections to the given address
// with an optional username and password. See RFC 1928.
func NewSOCKS5(s string, dialer proxy.Dialer) (*SOCKS5, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}

	h := &SOCKS5{
		dialer:   dialer,
		addr:     addr,
		user:     user,
		password: pass,
	}

	return h, nil
}

// NewSocks5Dialer returns a socks5 proxy dialer.
func NewSocks5Dialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewSOCKS5(s, dialer)
}

// NewSocks5Server returns a socks5 proxy server.
func NewSocks5Server(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewSOCKS5(s, dialer)
}

// ListenAndServe serves socks5 requests.
func (s *SOCKS5) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP .
func (s *SOCKS5) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("proxy-socks5 failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("proxy-socks5 listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("proxy-socks5 failed to accept: %v", err)
			continue
		}

		go s.ServeTCP(c)
	}
}

// ServeTCP .
func (s *SOCKS5) ServeTCP(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	tgt, err := s.handshake(c)
	if err != nil {
		// UDP: keep the connection until disconnect then free the UDP socket
		if err == socks.Errors[9] {
			buf := []byte{}
			// block here
			for {
				_, err := c.Read(buf)
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				// log.F("proxy-socks5 servetcp udp associate end")
				return
			}
		}

		log.F("proxy-socks5 failed to get target address: %v", err)
		return
	}

	rc, err := s.dialer.Dial("tcp", tgt.String())
	if err != nil {
		log.F("proxy-socks5 failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	log.F("proxy-socks5 %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("proxy-socks5 relay error: %v", err)
	}
}

// ListenAndServeUDP serves udp requests.
func (s *SOCKS5) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("proxy-socks5-udp failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("proxy-socks5-udp listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		c := NewSocks5PktConn(lc, nil, nil, true, nil)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("proxy-socks5-udp remote read error: %v", err)
			continue
		}

		var pc *Socks5PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			if c.tgtAddr == nil {
				log.F("proxy-socks5-udp can not get target address, not a valid request")
				continue
			}

			lpc, nextHop, err := s.dialer.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("proxy-socks5-udp remote dial error: %v", err)
				continue
			}

			pc = NewSocks5PktConn(lpc, nextHop, nil, false, nil)
			nm.Store(raddr.String(), pc)

			go func() {
				conn.TimedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(*Socks5PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("proxy-socks5-udp remote write error: %v", err)
			continue
		}

		log.F("proxy-socks5-udp %s <-> %s", raddr, c.tgtAddr)
	}

}

// Addr returns forwarder's address
func (s *SOCKS5) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *SOCKS5) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the SOCKS5 proxy.
func (s *SOCKS5) Dial(network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return nil, errors.New("proxy-socks5: no support for connection type " + network)
	}

	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	if err := s.connect(c, addr); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// DialUDP connects to the given address via the proxy.
func (s *SOCKS5) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("proxy-socks5 dialudp dial tcp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	// send VER, NMETHODS, METHODS
	c.Write([]byte{5, 1, 0})

	buf := make([]byte, socks.MaxAddrLen)
	// read VER METHOD
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return nil, nil, err
	}

	dstAddr := socks.ParseAddr(addr)
	// write VER CMD RSV ATYP DST.ADDR DST.PORT
	c.Write(append([]byte{5, socks.CmdUDPAssociate, 0}, dstAddr...))

	// read VER REP RSV ATYP BND.ADDR BND.PORT
	if _, err := io.ReadFull(c, buf[:3]); err != nil {
		return nil, nil, err
	}

	rep := buf[1]
	if rep != 0 {
		log.F("proxy-socks5 server reply: %d, not succeeded", rep)
		return nil, nil, errors.New("server connect failed")
	}

	uAddr, err := socks.ReadAddrBuf(c, buf)
	if err != nil {
		return nil, nil, err
	}

	pc, nextHop, err := s.dialer.DialUDP(network, uAddr.String())
	if err != nil {
		log.F("proxy-socks5 dialudp to %s error: %s", uAddr.String(), err)
		return nil, nil, err
	}

	pkc := NewSocks5PktConn(pc, nextHop, dstAddr, true, c)
	return pkc, nextHop, err
}

// connect takes an existing connection to a socks5 proxy server,
// and commands the server to extend that connection to target,
// which must be a canonical address with a host and port.
func (s *SOCKS5) connect(conn net.Conn, target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errors.New("proxy: failed to parse port number: " + portStr)
	}
	if port < 1 || port > 0xffff {
		return errors.New("proxy: port number out of range: " + portStr)
	}

	// the size here is just an estimate
	buf := make([]byte, 0, 6+len(host))

	buf = append(buf, Version)
	if len(s.user) > 0 && len(s.user) < 256 && len(s.password) < 256 {
		buf = append(buf, 2 /* num auth methods */, socks.AuthNone, socks.AuthPassword)
	} else {
		buf = append(buf, 1 /* num auth methods */, socks.AuthNone)
	}

	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write greeting to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read greeting from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}
	if buf[0] != 5 {
		return errors.New("proxy: SOCKS5 proxy at " + s.addr + " has unexpected version " + strconv.Itoa(int(buf[0])))
	}
	if buf[1] == 0xff {
		return errors.New("proxy: SOCKS5 proxy at " + s.addr + " requires authentication")
	}

	if buf[1] == socks.AuthPassword {
		buf = buf[:0]
		buf = append(buf, 1 /* password protocol version */)
		buf = append(buf, uint8(len(s.user)))
		buf = append(buf, s.user...)
		buf = append(buf, uint8(len(s.password)))
		buf = append(buf, s.password...)

		if _, err := conn.Write(buf); err != nil {
			return errors.New("proxy: failed to write authentication request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if _, err := io.ReadFull(conn, buf[:2]); err != nil {
			return errors.New("proxy: failed to read authentication reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if buf[1] != 0 {
			return errors.New("proxy: SOCKS5 proxy at " + s.addr + " rejected username/password")
		}
	}

	buf = buf[:0]
	buf = append(buf, Version, socks.CmdConnect, 0 /* reserved */)

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			buf = append(buf, socks.ATypeIP4)
			ip = ip4
		} else {
			buf = append(buf, socks.ATypeIP6)
		}
		buf = append(buf, ip...)
	} else {
		if len(host) > 255 {
			return errors.New("proxy: destination hostname too long: " + host)
		}
		buf = append(buf, socks.ATypeDomain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}
	buf = append(buf, byte(port>>8), byte(port))

	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write connect request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return errors.New("proxy: failed to read connect reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	failure := "unknown error"
	if int(buf[1]) < len(socks.Errors) {
		failure = socks.Errors[buf[1]].Error()
	}

	if len(failure) > 0 {
		return errors.New("proxy: SOCKS5 proxy at " + s.addr + " failed to connect: " + failure)
	}

	bytesToDiscard := 0
	switch buf[3] {
	case socks.ATypeIP4:
		bytesToDiscard = net.IPv4len
	case socks.ATypeIP6:
		bytesToDiscard = net.IPv6len
	case socks.ATypeDomain:
		_, err := io.ReadFull(conn, buf[:1])
		if err != nil {
			return errors.New("proxy: failed to read domain length from SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}
		bytesToDiscard = int(buf[0])
	default:
		return errors.New("proxy: got unknown address type " + strconv.Itoa(int(buf[3])) + " from SOCKS5 proxy at " + s.addr)
	}

	if cap(buf) < bytesToDiscard {
		buf = make([]byte, bytesToDiscard)
	} else {
		buf = buf[:bytesToDiscard]
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		return errors.New("proxy: failed to read address from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	// Also need to discard the port number
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read port from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	return nil
}

// Handshake fast-tracks SOCKS initialization to get target address to connect.
func (s *SOCKS5) handshake(rw io.ReadWriter) (socks.Addr, error) {
	// Read RFC 1928 for request and reply structure and sizes.
	buf := make([]byte, socks.MaxAddrLen)
	// read VER, NMETHODS, METHODS
	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return nil, err
	}
	nmethods := buf[1]
	if _, err := io.ReadFull(rw, buf[:nmethods]); err != nil {
		return nil, err
	}
	// write VER METHOD
	if _, err := rw.Write([]byte{5, 0}); err != nil {
		return nil, err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err := io.ReadFull(rw, buf[:3]); err != nil {
		return nil, err
	}
	cmd := buf[1]
	addr, err := socks.ReadAddrBuf(rw, buf)
	if err != nil {
		return nil, err
	}
	switch cmd {
	case socks.CmdConnect:
		_, err = rw.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) // SOCKS v5, reply succeeded
	case socks.CmdUDPAssociate:
		listenAddr := socks.ParseAddr(rw.(net.Conn).LocalAddr().String())
		_, err = rw.Write(append([]byte{5, 0, 0}, listenAddr...)) // SOCKS v5, reply succeeded
		if err != nil {
			return nil, socks.Errors[7]
		}
		err = socks.Errors[9]
	default:
		return nil, socks.Errors[7]
	}

	return addr, err // skip VER, CMD, RSV fields
}

// Socks5PktConn .
type Socks5PktConn struct {
	net.PacketConn

	writeAddr net.Addr // write to and read from addr

	tgtAddr   socks.Addr
	tgtHeader bool

	ctrlConn net.Conn // tcp control conn
}

// NewSocks5PktConn returns a Socks5PktConn
func NewSocks5PktConn(c net.PacketConn, writeAddr net.Addr, tgtAddr socks.Addr, tgtHeader bool, ctrlConn net.Conn) *Socks5PktConn {
	pc := &Socks5PktConn{
		PacketConn: c,
		writeAddr:  writeAddr,
		tgtAddr:    tgtAddr,
		tgtHeader:  tgtHeader,
		ctrlConn:   ctrlConn}

	if ctrlConn != nil {
		go func() {
			buf := []byte{}
			for {
				_, err := ctrlConn.Read(buf)
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				log.F("proxy-socks5 dialudp udp associate end")
				return
			}
		}()
	}

	return pc
}

// ReadFrom overrides the original function from net.PacketConn
func (pc *Socks5PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.ReadFrom(b)
	}

	buf := make([]byte, len(b))
	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, err
	}

	// https://tools.ietf.org/html/rfc1928#section-7
	// +----+------+------+----------+----------+----------+
	// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
	// +----+------+------+----------+----------+----------+
	// | 2  |  1   |  1   | Variable |    2     | Variable |
	// +----+------+------+----------+----------+----------+
	tgtAddr := socks.SplitAddr(buf[3:])
	copy(b, buf[3+len(tgtAddr):])

	//test
	if pc.writeAddr == nil {
		pc.writeAddr = raddr
	}

	if pc.tgtAddr == nil {
		pc.tgtAddr = tgtAddr
	}

	return n - len(tgtAddr) - 3, raddr, err
}

// WriteTo overrides the original function from net.PacketConn
func (pc *Socks5PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.WriteTo(b, addr)
	}

	buf := append([]byte{0, 0, 0}, pc.tgtAddr...)
	buf = append(buf, b[:]...)
	return pc.PacketConn.WriteTo(buf, pc.writeAddr)
}

// Close .
func (pc *Socks5PktConn) Close() error {
	if pc.ctrlConn != nil {
		pc.ctrlConn.Close()
	}

	return pc.PacketConn.Close()
}
