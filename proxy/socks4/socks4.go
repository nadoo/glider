// https://www.openssh.com/txt/socks4.protocol

// socks4 client

package socks4

import (
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

const (
	// Version is socks4 version number.
	Version = 4
	// ConnectCommand connect command byte
	ConnectCommand = 1
)

// SOCKS4 is a base socks4 struct.
type SOCKS4 struct {
	dialer  proxy.Dialer
	addr    string
	socks4a bool
}

func init() {
	proxy.RegisterDialer("socks4", NewSocks4Dialer)
	proxy.RegisterDialer("socks4a", NewSocks4Dialer)
}

// NewSOCKS4 returns a socks4 proxy.
func NewSOCKS4(s string, dialer proxy.Dialer) (*SOCKS4, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	h := &SOCKS4{
		dialer:  dialer,
		addr:    u.Host,
		socks4a: u.Scheme == "socks4a",
	}

	return h, nil
}

// NewSocks4Dialer returns a socks4 proxy dialer.
func NewSocks4Dialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewSOCKS4(s, dialer)
}

// Addr returns forwarder's address.
func (s *SOCKS4) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the SOCKS4 proxy.
func (s *SOCKS4) Dial(network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4":
	default:
		return nil, errors.New("[socks4] no support for connection type " + network)
	}

	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[socks4] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if err := s.connect(c, addr); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// DialUDP connects to the given address via the proxy.
func (s *SOCKS4) DialUDP(network, addr string) (pc net.PacketConn, err error) {
	return nil, proxy.ErrNotSupported
}

func (s *SOCKS4) lookupIP(host string) (ip net.IP, err error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return
	}
	if len(ips) == 0 {
		err = errors.New("[socks4] Cannot resolve host: " + host)
		return
	}
	ip = ips[0].To4()
	if len(ip) != net.IPv4len {
		err = errors.New("[socks4] IPv6 is not supported by socks4")
		return
	}
	return
}

// connect takes an existing connection to a socks4 proxy server,
// and commands the server to extend that connection to target,
// which must be a canonical address with a host and port.
func (s *SOCKS4) connect(conn net.Conn, target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return errors.New("[socks4] failed to parse port number: " + portStr)
	}

	const baseBufSize = 8 + 1 // 1 is the len(userid)
	bufSize := baseBufSize
	var ip net.IP
	if ip = net.ParseIP(host); ip == nil {
		if s.socks4a {
			// The client should set the first three bytes of DSTIP to NULL
			// and the last byte to a non-zero value.
			ip = []byte{0, 0, 0, 1}
			bufSize += len(host) + 1
		} else {
			ip, err = s.lookupIP(host)
			if err != nil {
				return err
			}
		}
	} else {
		ip = ip.To4()
		if ip == nil {
			return errors.New("[socks4] IPv6 is not supported by socks4")
		}
	}
	// taken from https://github.com/h12w/socks/blob/master/socks.go and https://en.wikipedia.org/wiki/SOCKS
	buf := pool.GetBuffer(bufSize)
	defer pool.PutBuffer(buf)
	copy(buf, []byte{
		Version,
		ConnectCommand,
		byte(port >> 8), // higher byte of destination port
		byte(port),      // lower byte of destination port (big endian)
		ip[0], ip[1], ip[2], ip[3],
		0, // user id
	})
	if s.socks4a {
		copy(buf[baseBufSize:], host)
		buf[len(buf)-1] = 0
	}

	resp := pool.GetBuffer(8)
	defer pool.PutBuffer(resp)

	if _, err := conn.Write(buf); err != nil {
		return errors.New("[socks4] failed to write greeting to socks4 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, resp); err != nil {
		return errors.New("[socks4] failed to read greeting from socks4 proxy at " + s.addr + ": " + err.Error())
	}

	switch resp[1] {
	case 0x5a:
		// request granted
	case 0x5b:
		err = errors.New("[socks4] connection request rejected or failed")
	case 0x5c:
		err = errors.New("[socks4] connection request request failed because client is not running identd (or not reachable from the server)")
	case 0x5d:
		err = errors.New("[socks4] connection request request failed because client's identd could not confirm the user ID in the request")
	default:
		err = errors.New("[socks4] connection request failed, unknown error")
	}

	return err
}

func init() {
	proxy.AddUsage("socks4", `
Socks4 scheme:
  socks4://host:port
`)
}
