package ssh

import (
	"errors"
	"io/ioutil"
	"net"
	"net/url"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

type SSH struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string
	config *ssh.ClientConfig
}

func init() {
	proxy.RegisterDialer("ssh", NewSSHDialer)
}

// NewSS returns a ssh proxy.
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

	ssh := &SSH{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
		config: config,
	}

	return ssh, nil
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
	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[ssh]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	sshc, ch, req, err := ssh.NewClientConn(c, s.addr, s.config)
	if err != nil {
		log.F("[ssh]: initial connection to %s error: %s", s.addr, err)
		return nil, err
	}

	return ssh.NewClient(sshc, ch, req).Dial(network, addr)
}

// DialUDP connects to the given address via the proxy.
func (s *SSH) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return nil, nil, errors.New("ssh client does not support udp")
}

func privateKeyAuth(file string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(key), nil
}
