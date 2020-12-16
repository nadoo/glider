package socks5

import (
	"errors"
	"io"
	"net"
	"strconv"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

// NewSocks5Dialer returns a socks5 proxy dialer.
func NewSocks5Dialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewSocks5(s, d, nil)
}

// Addr returns forwarder's address.
func (s *Socks5) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the SOCKS5 proxy.
func (s *Socks5) Dial(network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return nil, errors.New("[socks5]: no support for connection type " + network)
	}

	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[socks5]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if err := s.connect(c, addr); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// DialUDP connects to the given address via the proxy.
func (s *Socks5) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[socks5] dialudp dial tcp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	// send VER, NMETHODS, METHODS
	c.Write([]byte{Version, 1, 0})

	buf := pool.GetBuffer(socks.MaxAddrLen)
	defer pool.PutBuffer(buf)

	// read VER METHOD
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return nil, nil, err
	}

	dstAddr := socks.ParseAddr(addr)
	// write VER CMD RSV ATYP DST.ADDR DST.PORT
	c.Write(append([]byte{Version, socks.CmdUDPAssociate, 0}, dstAddr...))

	// read VER REP RSV ATYP BND.ADDR BND.PORT
	if _, err := io.ReadFull(c, buf[:3]); err != nil {
		return nil, nil, err
	}

	rep := buf[1]
	if rep != 0 {
		log.F("[socks5] server reply: %d, not succeeded", rep)
		return nil, nil, errors.New("server connect failed")
	}

	uAddr, err := socks.ReadAddrBuf(c, buf)
	if err != nil {
		return nil, nil, err
	}

	var uAddress string
	h, p, _ := net.SplitHostPort(uAddr.String())
	// if returned bind ip is unspecified
	if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
		// indicate using conventional addr
		uAddress = net.JoinHostPort(s.addr, p)
	} else {
		uAddress = uAddr.String()
	}

	pc, nextHop, err := s.dialer.DialUDP(network, uAddress)
	if err != nil {
		log.F("[socks5] dialudp to %s error: %s", uAddr.String(), err)
		return nil, nil, err
	}

	pkc := NewPktConn(pc, nextHop, dstAddr, true, c)
	return pkc, nextHop, err
}

// connect takes an existing connection to a socks5 proxy server,
// and commands the server to extend that connection to target,
// which must be a canonical address with a host and port.
func (s *Socks5) connect(conn net.Conn, target string) error {
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
	if buf[0] != Version {
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
			buf = append(buf, socks.ATypIP4)
			ip = ip4
		} else {
			buf = append(buf, socks.ATypIP6)
		}
		buf = append(buf, ip...)
	} else {
		if len(host) > 255 {
			return errors.New("proxy: destination hostname too long: " + host)
		}
		buf = append(buf, socks.ATypDomain)
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
	case socks.ATypIP4:
		bytesToDiscard = net.IPv4len
	case socks.ATypIP6:
		bytesToDiscard = net.IPv6len
	case socks.ATypDomain:
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
