package ssh

import (
	"net"
	"net/url"
	"os"
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

	mu      sync.Mutex
	sshCfg  *ssh.ClientConfig
	sshConn ssh.Conn
	sshChan <-chan ssh.NewChannel
	sshReq  <-chan *ssh.Request
}

func init() {
	proxy.RegisterDialer("ssh", NewSSHDialer)
}

// NewSSH returns a ssh proxy.
func NewSSH(s string, d proxy.Dialer, p proxy.Proxy) (*SSH, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	user := u.User.Username()
	if user == "" {
		user = "root"
	}

	config := &ssh.ClientConfig{
		User:    user,
		Timeout: time.Second * 3,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	if pass, _ := u.User.Password(); pass != "" {
		config.Auth = []ssh.AuthMethod{ssh.Password(pass)}
	}

	if key := u.Query().Get("key"); key != "" {
		keyAuth, err := privateKeyAuth(key)
		if err != nil {
			log.F("[ssh] read key file error: %s", err)
			return nil, err
		}
		config.Auth = append(config.Auth, keyAuth)
	}

	t := &SSH{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
		sshCfg: config,
	}

	if _, port, _ := net.SplitHostPort(t.addr); port == "" {
		t.addr = net.JoinHostPort(t.addr, "22")
	}

	return t, t.initConn()
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

func (s *SSH) initConn() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[ssh]: dial to %s error: %s", s.addr, err)
		return err
	}

	s.sshConn, s.sshChan, s.sshReq, err = ssh.NewClientConn(c, s.addr, s.sshCfg)
	if err != nil {
		log.F("[ssh]: initial connection to %s error: %s", s.addr, err)
		return err
	}

	return nil
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SSH) Dial(network, addr string) (net.Conn, error) {
	if c, err := ssh.NewClient(s.sshConn, s.sshChan, s.sshReq).Dial(network, addr); err == nil {
		return c, nil
	}
	s.sshConn.Close()
	if err := s.initConn(); err != nil {
		return nil, err
	}
	return ssh.NewClient(s.sshConn, s.sshChan, s.sshReq).Dial(network, addr)
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
