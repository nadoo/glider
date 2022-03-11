package smux

import (
	"net"
	"net/url"
	"sync"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/smux"
	"github.com/nadoo/glider/proxy"
)

// SmuxClient struct.
type SmuxClient struct {
	dialer  proxy.Dialer
	addr    string
	mu      sync.Mutex
	session *smux.Session
}

func init() {
	proxy.RegisterDialer("smux", NewSmuxDialer)
}

// NewSmuxDialer returns a smux dialer.
func NewSmuxDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[smux] parse url err: %s", err)
		return nil, err
	}

	c := &SmuxClient{
		dialer: d,
		addr:   u.Host,
	}

	return c, nil
}

// Addr returns forwarder's address.
func (s *SmuxClient) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SmuxClient) Dial(network, addr string) (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		if c, err := s.session.OpenStream(); err == nil {
			return c, err
		}
		s.session.Close()
	}
	if err := s.initConn(); err != nil {
		return nil, err
	}
	return s.session.OpenStream()
}

// DialUDP connects to the given address via the proxy.
func (s *SmuxClient) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func (s *SmuxClient) initConn() error {
	conn, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[smux] dial to %s error: %s", s.addr, err)
		return err
	}
	s.session, err = smux.Client(conn, nil)
	return err
}
