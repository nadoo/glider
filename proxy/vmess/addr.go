package vmess

import (
	"net"
	"strconv"
)

// AType is vmess addr type
type AType byte

// Atyp
const (
	ATypeErr    AType = 0
	ATypeIP4    AType = 1
	ATypeDomain AType = 2
	ATypeIP6    AType = 3
)

// Addr is vmess addr
type Addr []byte

// Port is vmess addr port
type Port uint16

// ParseAddr parses the address in string s. return AType = 0 if error.
func ParseAddr(s string) (AType, Addr, Port, error) {
	var atype AType
	var addr Addr

	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return 0, nil, 0, err
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			addr = make([]byte, net.IPv4len)
			atype = ATypeIP4
			copy(addr[:], ip4)
		} else {
			addr = make([]byte, net.IPv6len)
			atype = ATypeIP6
			copy(addr[:], ip)
		}
	} else {
		if len(host) > 255 {
			return 0, nil, 0, err
		}
		addr = make([]byte, 1+len(host))
		atype = ATypeDomain
		addr[0] = byte(len(host))
		copy(addr[1:], host)
	}

	portnum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, nil, 0, err
	}

	return atype, addr, Port(portnum), err
}
