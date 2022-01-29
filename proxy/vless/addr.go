package vless

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"

	"github.com/nadoo/glider/pkg/pool"
)

// Atyp is vless addr type.
type Atyp byte

// Atyp
const (
	AtypErr    Atyp = 0
	AtypIP4    Atyp = 1
	AtypDomain Atyp = 2
	AtypIP6    Atyp = 3
)

// Addr is vless addr.
type Addr []byte

// MaxHostLen is the maximum size of host in bytes.
const MaxHostLen = 255

// Port is vless addr port.
type Port uint16

// ParseAddr parses the address in string s.
func ParseAddr(s string) (Atyp, Addr, Port, error) {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return 0, nil, 0, err
	}

	var addr Addr
	var atyp Atyp = AtypIP4
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is6() {
			atyp = AtypIP6
		}
		addr = ip.AsSlice()
	} else {
		if len(host) > MaxHostLen {
			return 0, nil, 0, err
		}
		addr = make([]byte, 1+len(host))
		atyp = AtypDomain
		addr[0] = byte(len(host))
		copy(addr[1:], host)
	}

	portnum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, nil, 0, err
	}

	return atyp, addr, Port(portnum), err
}

// ReadAddr reads just enough bytes from r to get addr.
func ReadAddr(r io.Reader) (atyp Atyp, host Addr, port Port, err error) {
	buf := pool.GetBuffer(2)
	defer pool.PutBuffer(buf)

	// port
	_, err = io.ReadFull(r, buf[:2])
	if err != nil {
		return
	}
	port = Port(binary.BigEndian.Uint16(buf[:2]))

	// atyp
	_, err = io.ReadFull(r, buf[:1])
	if err != nil {
		return
	}
	atyp = Atyp(buf[0])

	switch atyp {
	case AtypIP4:
		host = make([]byte, net.IPv4len)
		_, err = io.ReadFull(r, host)
		return
	case AtypIP6:
		host = make([]byte, net.IPv6len)
		_, err = io.ReadFull(r, host)
		return
	case AtypDomain:
		_, err = io.ReadFull(r, buf[:1])
		if err != nil {
			return
		}
		host = make([]byte, int(buf[0]))
		_, err = io.ReadFull(r, host)
		return
	}

	return
}

// ReadAddrString reads just enough bytes from r to get addr string.
func ReadAddrString(r io.Reader) (string, error) {
	atyp, host, port, err := ReadAddr(r)
	if err != nil {
		return "", err
	}
	return AddrString(atyp, host, port), nil
}

// AddrString returns a addr string in format of "host:port".
func AddrString(atyp Atyp, addr Addr, port Port) string {
	var host string

	switch atyp {
	case AtypIP4, AtypIP6:
		host = net.IP(addr).String()
	case AtypDomain:
		host = string(addr)
	}

	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}
