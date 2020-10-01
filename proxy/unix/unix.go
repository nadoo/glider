package unix

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// Unix domain socket struct.
type Unix struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	server proxy.Server
}

func init() {
	proxy.RegisterServer("unix", NewUnixServer)
	proxy.RegisterDialer("unix", NewUnixDialer)
}

// NewUnix returns  unix fomain socket proxy.
func NewUnix(s string, d proxy.Dialer, p proxy.Proxy) (*Unix, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	unix := &Unix{
		dialer: d,
		proxy:  p,
		addr:   u.Path,
	}

	return unix, nil
}

// NewUnixDialer returns a unix domain socket dialer.
func NewUnixDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewUnix(s, d, nil)
}

// NewUnixServer returns a unix domain socket server.
func NewUnixServer(s string, p proxy.Proxy) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	// TODO: check here
	if len(transport) < 2 {
		return nil, errors.New("[unix] malformd listener:" + s)
	}

	unix, err := NewUnix(transport[0], nil, p)
	if err != nil {
		return nil, err
	}

	unix.server, err = proxy.ServerFromURL(transport[1], p)
	if err != nil {
		return nil, err
	}

	return unix, nil
}

// ListenAndServe serves requests.
func (s *Unix) ListenAndServe() {
	os.Remove(s.addr)
	l, err := net.Listen("unix", s.addr)
	if err != nil {
		log.F("[unix] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[unix] listening on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[unix] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests.
func (s *Unix) Serve(c net.Conn) {
	// we know the internal server will close the connection after serve
	// defer c.Close()

	if s.server != nil {
		s.server.Serve(c)
	}
}

// Addr returns forwarder's address.
func (s *Unix) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *Unix) Dial(network, addr string) (net.Conn, error) {
	// NOTE: must be the first dialer in a chain
	return net.Dial("unix", s.addr)
}

// DialUDP connects to the given address via the proxy.
func (s *Unix) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("unix domain socket client does not support udp now")
}
