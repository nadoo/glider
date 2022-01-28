package redir

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/pkg/log"
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
		log.Fatalf("[redir] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[redir] listening TCP on " + s.addr)

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
	tgtAddr, err := getOrigDst(c, s.ipv6)
	if err != nil {
		log.F("[redir] failed to get target address: %v", err)
		return
	}
	tgt := tgtAddr.String()

	// loop request
	if c.LocalAddr().String() == tgt {
		log.F("[redir] %s <-> %s, unallowed request to redir port", c.RemoteAddr(), tgt)
		return
	}

	rc, dialer, err := s.proxy.Dial("tcp", tgt)
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
func getOrigDst(c *net.TCPConn, ipv6 bool) (netip.AddrPort, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return netip.AddrPort{}, err
	}
	var addr netip.AddrPort
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
func getorigdst(fd uintptr) (netip.AddrPort, error) {
	const _SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv4.h
	var raw syscall.RawSockaddrInet4
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IP, _SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return netip.AddrPort{}, err
	}
	// NOTE: raw.Port is big-endian, just change it to little-endian
	// TODO: improve here when we add big-endian $GOARCH support
	port := raw.Port<<8 | raw.Port>>8
	return netip.AddrPortFrom(netip.AddrFrom4(raw.Addr), port), nil
}

// Call ipv6_getorigdst() from linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c
func getorigdstIPv6(fd uintptr) (netip.AddrPort, error) {
	const _IP6T_SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
	var raw syscall.RawSockaddrInet6
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, _IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return netip.AddrPort{}, err
	}
	// NOTE: raw.Port is big-endian, just change it to little-endian
	// TODO: improve here when we add big-endian $GOARCH support
	port := raw.Port<<8 | raw.Port>>8
	return netip.AddrPortFrom(netip.AddrFrom16(raw.Addr), port), nil
}
