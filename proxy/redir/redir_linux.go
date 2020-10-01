// getOrigDst:
// https://github.com/shadowsocks/go-shadowsocks2/blob/master/tcp_linux.go#L30

package redir

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

const (
	// SO_ORIGINAL_DST from linux/include/uapi/linux/netfilter_ipv4.h
	SO_ORIGINAL_DST = 80
	// IP6T_SO_ORIGINAL_DST from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
	IP6T_SO_ORIGINAL_DST = 80
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
func (s *RedirProxy) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

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
func getOrigDst(conn net.Conn, ipv6 bool) (socks.Addr, error) {
	c, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("only work with TCP connection")
	}
	f, err := c.File()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fd := f.Fd()

	// The File() call above puts both the original socket fd and the file fd in blocking mode.
	// Set the file fd back to non-blocking mode and the original socket fd will become non-blocking as well.
	// Otherwise blocking I/O will waste OS threads.
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		return nil, err
	}

	if ipv6 {
		return getorigdstIPv6(fd)
	}

	return getorigdst(fd)
}

// Call getorigdst() from linux/net/ipv4/netfilter/nf_conntrack_l3proto_ipv4.c
func getorigdst(fd uintptr) (socks.Addr, error) {
	raw := syscall.RawSockaddrInet4{}
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}

	addr := make([]byte, 1+net.IPv4len+2)
	addr[0] = socks.ATypIP4
	copy(addr[1:1+net.IPv4len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv4len], addr[1+net.IPv4len+1] = port[0], port[1]
	return addr, nil
}

// Call ipv6_getorigdst() from linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c
func getorigdstIPv6(fd uintptr) (socks.Addr, error) {
	raw := syscall.RawSockaddrInet6{}
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}

	addr := make([]byte, 1+net.IPv6len+2)
	addr[0] = socks.ATypIP6
	copy(addr[1:1+net.IPv6len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv6len], addr[1+net.IPv6len+1] = port[0], port[1]
	return addr, nil
}
