package socks5

import (
	"errors"
	"io"
	"net"
	"strconv"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/protocol/socks"
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
	c, err := s.dial(network, s.addr)
	if err != nil {
		log.F("[socks5]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if _, err := s.connect(c, addr, socks.CmdConnect); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

func (s *Socks5) dial(network, addr string) (net.Conn, error) {
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

	return c, nil
}

// DialUDP connects to the given address via the proxy.
func (s *Socks5) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	c, err := s.dial("tcp", s.addr)
	if err != nil {
		log.F("[socks5] dialudp dial tcp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	var uAddr socks.Addr
	if uAddr, err = s.connect(c, addr, socks.CmdUDPAssociate); err != nil {
		c.Close()
		return nil, nil, err
	}

	buf := pool.GetBuffer(socks.MaxAddrLen)
	defer pool.PutBuffer(buf)

	var uAddress string
	h, p, _ := net.SplitHostPort(uAddr.String())
	// if returned bind ip is unspecified
	if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
		// indicate using conventional addr
		h, _, _ = net.SplitHostPort(s.addr)
		uAddress = net.JoinHostPort(h, p)
	} else {
		uAddress = uAddr.String()
	}

	pc, nextHop, err := s.dialer.DialUDP(network, uAddress)
	if err != nil {
		log.F("[socks5] dialudp to %s error: %s", uAddress, err)
		return nil, nil, err
	}

	pkc := NewPktConn(pc, nextHop, socks.ParseAddr(addr), true, c)
	return pkc, nextHop, err
}

// connect takes an existing connection to a socks5 proxy server,
// and commands the server to extend that connection to target,
// which must be a canonical address with a host and port.
func (s *Socks5) connect(conn net.Conn, target string, cmd byte) (addr socks.Addr, err error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return addr, errors.New("proxy: failed to parse port number: " + portStr)
	}
	if port < 1 || port > 0xffff {
		return addr, errors.New("proxy: port number out of range: " + portStr)
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
		return addr, errors.New("proxy: failed to write greeting to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return addr, errors.New("proxy: failed to read greeting from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}
	if buf[0] != Version {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " has unexpected version " + strconv.Itoa(int(buf[0])))
	}
	if buf[1] == 0xff {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " requires authentication")
	}

	if buf[1] == socks.AuthPassword {
		buf = buf[:0]
		buf = append(buf, 1 /* password protocol version */)
		buf = append(buf, uint8(len(s.user)))
		buf = append(buf, s.user...)
		buf = append(buf, uint8(len(s.password)))
		buf = append(buf, s.password...)

		if _, err := conn.Write(buf); err != nil {
			return addr, errors.New("proxy: failed to write authentication request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if _, err := io.ReadFull(conn, buf[:2]); err != nil {
			return addr, errors.New("proxy: failed to read authentication reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if buf[1] != 0 {
			return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " rejected username/password")
		}
	}

	buf = buf[:0]
	buf = append(buf, Version, cmd, 0 /* reserved */)

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
			return addr, errors.New("proxy: destination hostname too long: " + host)
		}
		buf = append(buf, socks.ATypDomain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}
	buf = append(buf, byte(port>>8), byte(port))

	if _, err := conn.Write(buf); err != nil {
		return addr, errors.New("proxy: failed to write connect request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	// read VER REP RSV
	if _, err := io.ReadFull(conn, buf[:3]); err != nil {
		return addr, errors.New("proxy: failed to read connect reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	failure := "unknown error"
	if int(buf[1]) < len(socks.Errors) {
		failure = socks.Errors[buf[1]].Error()
	}

	if len(failure) > 0 {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " failed to connect: " + failure)
	}

	return socks.ReadAddr(conn)
}
