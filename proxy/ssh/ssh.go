package ssh

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// SSH is a base ssh struct.
type SSH struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	mu     sync.Mutex
	conn   net.Conn
	client *ssh.Client
	config *ssh.ClientConfig
}

func init() {
	proxy.RegisterDialer("ssh", NewSSHDialer)
}

// NewSSH returns a ssh proxy.
func NewSSH(s string, d proxy.Dialer, p proxy.Proxy) (*SSH, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[ssh] parse err: %s", err)
		return nil, err
	}

	user := u.User.Username()
	if user == "" {
		user = "root"
	}

	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if pass, _ := u.User.Password(); pass != "" {
		config.Auth = []ssh.AuthMethod{ssh.Password(pass)}
	}

	query := u.Query()
	if key := query.Get("key"); key != "" {
		keyAuth, err := privateKeyAuth(key)
		if err != nil {
			log.F("[ssh] read key file error: %s", err)
			return nil, err
		}
		config.Auth = append(config.Auth, keyAuth)
	}

	// timeout of ssh handshake and channel operation
	qtimeout := query.Get("timeout")
	if qtimeout == "" {
		qtimeout = "5" // default timeout
	}
	timeout, err := strconv.ParseUint(qtimeout, 10, 32)
	if err != nil {
		log.F("[ssh] parse timeout err: %s", err)
		return nil, err
	}
	config.Timeout = time.Second * time.Duration(timeout)

	t := &SSH{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
		config: config,
	}

	if _, port, _ := net.SplitHostPort(t.addr); port == "" {
		t.addr = net.JoinHostPort(t.addr, "22")
	}

	return t, nil
}

// NewSSHDialer returns a ssh proxy dialer.
func NewSSHDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewSSH(s, d, nil)
}

// Addr returns forwarder's address.
func (s *SSH) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SSH) Dial(network, addr string) (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		if c, err := s.dial(network, addr); err == nil {
			return c, nil
		}
		s.conn.Close()
	}

	if err := s.initConn(); err != nil {
		return nil, err
	}

	return s.dial(network, addr)
}

func (s *SSH) dial(network, addr string) (net.Conn, error) {
	s.conn.SetDeadline(time.Now().Add(s.config.Timeout))
	c, err := s.client.Dial(network, addr)
	s.conn.SetDeadline(time.Time{})
	return c, err
}

func (s *SSH) initConn() error {
	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[ssh]: dial to %s error: %s", s.addr, err)
		return err
	}

	c.SetDeadline(time.Now().Add(s.config.Timeout))
	sshConn, sshChan, sshReq, err := ssh.NewClientConn(c, s.addr, s.config)
	if err != nil {
		log.F("[ssh]: initial connection to %s error: %s", s.addr, err)
		c.Close()
		return err
	}
	c.SetDeadline(time.Time{})

	s.conn = c
	s.client = ssh.NewClient(sshConn, sshChan, sshReq)
	return nil
}

// DialUDP connects to the given address via the proxy.
func (s *SSH) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return nil, nil, proxy.ErrNotSupported
}

func privateKeyAuth(file string) (ssh.AuthMethod, error) {
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(key), nil
}
