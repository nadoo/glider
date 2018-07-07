package vmess

import (
	"net"
	"strconv"
)

// Atyp is vmess addr type
type Atyp byte

// Atyp
const (
	AtypErr    Atyp = 0
	AtypIP4    Atyp = 1
	AtypDomain Atyp = 2
	AtypIP6    Atyp = 3
)

// Addr is vmess addr
type Addr []byte

// Port is vmess addr port
type Port uint16

// ParseAddr parses the address in string s
func ParseAddr(s string) (Atyp, Addr, Port, error) {
	var atyp Atyp
	var addr Addr

	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return 0, nil, 0, err
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			addr = make([]byte, net.IPv4len)
			atyp = AtypIP4
			copy(addr[:], ip4)
		} else {
			addr = make([]byte, net.IPv6len)
			atyp = AtypIP6
			copy(addr[:], ip)
		}
	} else {
		if len(host) > 255 {
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
