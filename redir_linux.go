// getOrigDst:
// https://github.com/shadowsocks/go-shadowsocks2/blob/master/tcp_linux.go#L30

package main

import (
	"errors"
	"net"
	"syscall"
	"unsafe"

	"github.com/shadowsocks/go-shadowsocks2/socks"
)

const (
	SO_ORIGINAL_DST      = 80 // from linux/include/uapi/linux/netfilter_ipv4.h
	IP6T_SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
)

type RedirProxy struct {
	*proxy
}

// NewRedirProxy returns a redirect proxy.
func NewRedirProxy(addr string, upProxy Proxy) (*RedirProxy, error) {
	s := &redir{
		proxy: newProxy(addr, upProxy),
	}

	return s, nil
}

// ListenAndServe redirected requests as a server.
func (s *RedirProxy) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}

	logf("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			tgt, err := getOrigDst(c, false)
			if err != nil {
				logf("failed to get target address: %v", err)
				return
			}

			rc, err := s.GetProxy(tgt.String()).Dial("tcp", tgt.String())
			if err != nil {
				logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			logf("proxy-redir %s <-> %s", c.RemoteAddr(), tgt)

			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}

		}()
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
		return ipv6_getorigdst(fd)
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
	addr[0] = socks.AtypIPv4
	copy(addr[1:1+net.IPv4len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv4len], addr[1+net.IPv4len+1] = port[0], port[1]
	return addr, nil
}

// Call ipv6_getorigdst() from linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c
// NOTE: I haven't tried yet but it should work since Linux 3.8.
func ipv6_getorigdst(fd uintptr) (socks.Addr, error) {
	raw := syscall.RawSockaddrInet6{}
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}

	addr := make([]byte, 1+net.IPv6len+2)
	addr[0] = socks.AtypIPv6
	copy(addr[1:1+net.IPv6len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv6len], addr[1+net.IPv6len+1] = port[0], port[1]
	return addr, nil
}
