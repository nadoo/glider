package unix

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// Unix domain socket struct
type Unix struct {
	dialer proxy.Dialer
	addr   string

	server proxy.Server
}

func init() {
	proxy.RegisterServer("unix", NewUnixServer)
	proxy.RegisterDialer("unix", NewUnixDialer)
}

// NewUnix returns  unix fomain socket proxy
func NewUnix(s string, dialer proxy.Dialer) (*Unix, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	p := &Unix{
		dialer: dialer,
		addr:   u.Path,
	}

	return p, nil
}

// NewUnixDialer returns a unix domain socket dialer
func NewUnixDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewUnix(s, dialer)
}

// NewUnixServer returns a unix domain socket server
func NewUnixServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	// TODO: check here
	if len(transport) < 2 {
		return nil, errors.New("[unix] malformd listener:" + s)
	}

	p, err := NewUnix(transport[0], dialer)
	if err != nil {
		return nil, err
	}

	p.server, err = proxy.ServerFromURL(transport[1], dialer)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// ListenAndServe serves requests
func (s *Unix) ListenAndServe() {
	os.Remove(s.addr)
	l, err := net.Listen("unix", s.addr)
	if err != nil {
		log.F("[unix] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[uinx] listening on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[uinx] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests
func (s *Unix) Serve(c net.Conn) {
	// we know the internal server will close the connection after serve
	// defer c.Close()

	if s.server != nil {
		s.server.Serve(c)
	}
}

// Addr returns forwarder's address
func (s *Unix) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// NextDialer returns the next dialer
func (s *Unix) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *Unix) Dial(network, addr string) (net.Conn, error) {
	// NOTE: must be the first dialer in a chain
	rc, err := net.Dial("unix", s.addr)
	if err != nil {
		return nil, err
	}

	return rc, err
}

// DialUDP connects to the given address via the proxy
func (s *Unix) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("unix domain socket client does not support udp now")
}
