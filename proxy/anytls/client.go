package anytls

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("anytls", NewAnyTLSDialer)
	proxy.RegisterDialer("anytlsc", NewClearTextDialer)
}

func NewAnyTLSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	a, err := NewAnyTLS(s, d, nil)
	if err != nil {
		return nil, fmt.Errorf("[anytls] create instance error: %s", err)
	}
	a.tlsConfig, err = loadClientTLSConfig(a.serverName, a.certFile, a.skipVerify)
	return a, err
}

func NewClearTextDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	a, err := NewAnyTLS(s, d, nil)
	if err != nil {
		return nil, fmt.Errorf("[anytlsc] create instance error: %s", err)
	}
	a.withTLS = false
	return a, nil
}

func (s *AnyTLS) Dial(network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, proxy.ErrNotSupported
	}
	raw := socks.ParseAddr(addr)
	if raw == nil {
		return nil, fmt.Errorf("[anytls] invalid target address: %s", addr)
	}
	ss, err := s.newClientSession()
	if err != nil {
		return nil, err
	}
	st, err := ss.openStream()
	if err != nil {
		_ = ss.Close()
		return nil, err
	}
	if _, err := st.Write(raw); err != nil {
		_ = st.Close()
		_ = ss.Close()
		return nil, err
	}
	if err := ss.waitSYNACK(st.id, s.synackTimeout); err != nil {
		_ = st.Close()
		_ = ss.Close()
		return nil, err
	}
	return &clientConn{stream: st}, nil
}

func (s *AnyTLS) DialUDP(network, addr string) (net.PacketConn, error) {
	if network != "udp" && network != "udp4" && network != "udp6" {
		return nil, proxy.ErrNotSupported
	}
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, fmt.Errorf("[anytls] invalid target address: %s", addr)
	}
	raw := socks.ParseAddr(net.JoinHostPort(uotV2MagicHost, "0"))
	if raw == nil {
		return nil, fmt.Errorf("[anytls] invalid udp-over-tcp target address")
	}
	ss, err := s.newClientSession()
	if err != nil {
		return nil, err
	}
	st, err := ss.openStream()
	if err != nil {
		_ = ss.Close()
		return nil, err
	}
	if _, err := st.Write(raw); err != nil {
		_ = st.Close()
		_ = ss.Close()
		return nil, err
	}
	if err := writeUOTV2Request(st, target); err != nil {
		_ = st.Close()
		_ = ss.Close()
		return nil, err
	}
	if err := ss.waitSYNACK(st.id, s.synackTimeout); err != nil {
		_ = st.Close()
		_ = ss.Close()
		return nil, err
	}
	return &clientPacketConn{PacketConn: newUOTPacketConn(st, target), stream: st}, nil
}

func (s *AnyTLS) newClientSession() (*session, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[anytls] dial to %s error: %s", s.addr, err)
		return nil, err
	}
	c := rc
	if s.withTLS {
		tc := tls.Client(rc, s.tlsConfig)
		if err := tc.Handshake(); err != nil {
			_ = rc.Close()
			return nil, err
		}
		c = tc
	}
	if err := writeAuth(c, s.password, s.padding.authPaddingLen()); err != nil {
		_ = c.Close()
		return nil, err
	}
	ss := newSession(c)
	if err := ss.writeFrame(frame{command: cmdSettings, data: clientSettings(s.padding)}); err != nil {
		_ = ss.Close()
		return nil, err
	}
	ss.start()
	return ss, nil
}

type clientConn struct {
	*stream
}

func (c *clientConn) Close() error {
	return closeClientSession(c.stream)
}

type clientPacketConn struct {
	net.PacketConn
	stream *stream
}

func (c *clientPacketConn) Close() error {
	return closeClientSession(c.stream)
}

const clientFINTimeout = time.Second

// The client owns the whole session. Give FIN a chance to reach the transport
// before tearing it down, while keeping standalone stream.Close nonblocking.
func closeClientSession(st *stream) error {
	_ = st.Close()
	defer st.s.Close()
	if st.fin != nil {
		timer := time.NewTimer(clientFINTimeout)
		defer timer.Stop()
		select {
		case <-st.fin.result:
		case <-st.s.done:
		case <-timer.C:
		}
	}
	return nil
}
