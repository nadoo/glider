package redir

import (
	"net"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// RedirProxy struct.
type RedirProxy struct {
	proxy proxy.Proxy
	addr  string
	ipv6  bool
}

func init() {
	proxy.RegisterServer("redir", NewRedirServer)
	proxy.RegisterServer("redir6", NewRedir6Server)
}

// NewRedirProxy returns a redirect proxy.
func NewRedirProxy(s string, p proxy.Proxy, ipv6 bool) (*RedirProxy, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	r := &RedirProxy{
		proxy: p,
		addr:  addr,
		ipv6:  ipv6,
	}

	return r, nil
}

// NewRedirServer returns a redir server.
func NewRedirServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewRedirProxy(s, p, false)
}

// NewRedir6Server returns a redir server for ipv6.
func NewRedir6Server(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewRedirProxy(s, p, true)
}

// ListenAndServe listens on server's addr and serves connections.
func (s *RedirProxy) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[redir] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[redir] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[redir] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves connections.
func (s *RedirProxy) Serve(cc net.Conn) {
	defer cc.Close()

	c, ok := cc.(*net.TCPConn)
	if !ok {
		log.F("[redir] not a tcp connection, can not chain redir proxy")
		return
	}

	c.SetKeepAlive(true)
	tgt, err := getOrigDst(c, s.ipv6)
	if err != nil {
		log.F("[redir] failed to get target address: %v", err)
		return
	}

	// loop request
	if c.LocalAddr().String() == tgt.String() {
		log.F("[redir] %s <-> %s, unallowed request to redir port", c.RemoteAddr(), tgt)
		return
	}

	rc, dialer, err := s.proxy.Dial("tcp", tgt.String())
	if err != nil {
		log.F("[redir] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[redir] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[redir] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// Get the original destination of a TCP connection.
func getOrigDst(c *net.TCPConn, ipv6 bool) (*net.TCPAddr, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var addr *net.TCPAddr
	rc.Control(func(fd uintptr) {
		if ipv6 {
			addr, err = getorigdstIPv6(fd)
		} else {
			addr, err = getorigdst(fd)
		}
	})
	return addr, err
}

// Call getorigdst() from linux/net/ipv4/netfilter/nf_conntrack_l3proto_ipv4.c
func getorigdst(fd uintptr) (*net.TCPAddr, error) {
	const _SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv4.h
	var raw syscall.RawSockaddrInet4
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IP, _SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}
	var addr net.TCPAddr
	addr.IP = raw.Addr[:]
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // raw.Port is big-endian
	addr.Port = int(port[0])<<8 | int(port[1])
	return &addr, nil
}

// Call ipv6_getorigdst() from linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c
// NOTE: I haven't tried yet but it should work since Linux 3.8.
func getorigdstIPv6(fd uintptr) (*net.TCPAddr, error) {
	const _IP6T_SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
	var raw syscall.RawSockaddrInet6
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, _IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}
	var addr net.TCPAddr
	addr.IP = raw.Addr[:]
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // raw.Port is big-endian
	addr.Port = int(port[0])<<8 | int(port[1])
	return &addr, nil
}
