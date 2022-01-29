package vmess

import (
	"net"
	"net/netip"
	"strconv"
)

// Atyp is vmess addr type.
type Atyp byte

// Atyp
const (
	AtypErr    Atyp = 0
	AtypIP4    Atyp = 1
	AtypDomain Atyp = 2
	AtypIP6    Atyp = 3
)

// Addr is vmess addr.
type Addr []byte

// MaxHostLen is the maximum size of host in bytes.
const MaxHostLen = 255

// Port is vmess addr port.
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
