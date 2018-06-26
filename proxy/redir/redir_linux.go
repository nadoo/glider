// getOrigDst:
// https://github.com/shadowsocks/go-shadowsocks2/blob/master/tcp_linux.go#L30

package redir

import (
	"errors"
	"net"
	"net/url"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

const (
	// SO_ORIGINAL_DST from linux/include/uapi/linux/netfilter_ipv4.h
	SO_ORIGINAL_DST = 80
	// IP6T_SO_ORIGINAL_DST from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
	IP6T_SO_ORIGINAL_DST = 80
)

// RedirProxy struct
type RedirProxy struct {
	dialer proxy.Dialer
	addr   string
}

func init() {
	proxy.RegisterServer("redir", NewRedirServer)
}

// NewRedirProxy returns a redirect proxy.
func NewRedirProxy(s string, dialer proxy.Dialer) (*RedirProxy, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	r := &RedirProxy{
		dialer: dialer,
		addr:   addr,
	}

	return r, nil
}

// NewRedirServer returns a redir server.
func NewRedirServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewRedirProxy(s, dialer)
}

// ListenAndServe .
func (s *RedirProxy) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("proxy-redir failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("proxy-redir listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("proxy-redir failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			tgt, err := getOrigDst(c, false)
			if err != nil {
				log.F("proxy-redir failed to get target address: %v", err)
				return
			}

			rc, err := s.dialer.Dial("tcp", tgt.String())
			if err != nil {
				log.F("proxy-redir failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			log.F("proxy-redir %s <-> %s", c.RemoteAddr(), tgt)

			_, _, err = conn.Relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				log.F("proxy-redir relay error: %v", err)
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
	addr[0] = socks.ATypeIP4
	copy(addr[1:1+net.IPv4len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv4len], addr[1+net.IPv4len+1] = port[0], port[1]
	return addr, nil
}

// Call ipv6_getorigdst() from linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c
// NOTE: I haven't tried yet but it should work since Linux 3.8.
func getorigdstIPv6(fd uintptr) (socks.Addr, error) {
	raw := syscall.RawSockaddrInet6{}
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return nil, err
	}

	addr := make([]byte, 1+net.IPv6len+2)
	addr[0] = socks.ATypeIP6
	copy(addr[1:1+net.IPv6len], raw.Addr[:])
	port := (*[2]byte)(unsafe.Pointer(&raw.Port)) // big-endian
	addr[1+net.IPv6len], addr[1+net.IPv6len+1] = port[0], port[1]
	return addr, nil
}
