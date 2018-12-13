package kcp

import (
	"crypto/sha1"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	kcp "github.com/xtaci/kcp-go"
	"golang.org/x/crypto/pbkdf2"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// KCP struct
type KCP struct {
	dialer proxy.Dialer
	addr   string

	key   string
	crypt string
	block kcp.BlockCrypt

	dataShards   int
	parityShards int

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("kcp", NewKCPDialer)
	proxy.RegisterServer("kcp", NewKCPServer)
}

// NewKCP returns a kcp proxy struct
func NewKCP(s string, dialer proxy.Dialer) (*KCP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[kcp] parse url err: %s", err)
		return nil, err
	}

	addr := u.Host
	crypt := u.User.Username()
	key, _ := u.User.Password()

	query := u.Query()

	// dataShards
	dShards := query.Get("dataShards")
	if dShards == "" {
		dShards = "10"
	}

	dataShards, err := strconv.ParseUint(dShards, 10, 32)
	if err != nil {
		log.F("[kcp] parse dataShards err: %s", err)
		return nil, err
	}

	// parityShards
	pShards := query.Get("parityShards")
	if pShards == "" {
		pShards = "3"
	}

	parityShards, err := strconv.ParseUint(pShards, 10, 32)
	if err != nil {
		log.F("[kcp] parse parityShards err: %s", err)
		return nil, err
	}

	p := &KCP{
		dialer:       dialer,
		addr:         addr,
		key:          key,
		crypt:        crypt,
		dataShards:   int(dataShards),
		parityShards: int(parityShards),
	}

	if p.crypt != "" {
		pass := pbkdf2.Key([]byte(p.key), []byte("kcp-go"), 4096, 32, sha1.New)
		var block kcp.BlockCrypt
		switch p.crypt {
		case "sm4":
			block, _ = kcp.NewSM4BlockCrypt(pass[:16])
		case "tea":
			block, _ = kcp.NewTEABlockCrypt(pass[:16])
		case "xor":
			block, _ = kcp.NewSimpleXORBlockCrypt(pass)
		case "none":
			block, _ = kcp.NewNoneBlockCrypt(pass)
		case "aes":
			block, _ = kcp.NewAESBlockCrypt(pass)
		case "aes-128":
			block, _ = kcp.NewAESBlockCrypt(pass[:16])
		case "aes-192":
			block, _ = kcp.NewAESBlockCrypt(pass[:24])
		case "blowfish":
			block, _ = kcp.NewBlowfishBlockCrypt(pass)
		case "twofish":
			block, _ = kcp.NewTwofishBlockCrypt(pass)
		case "cast5":
			block, _ = kcp.NewCast5BlockCrypt(pass[:16])
		case "3des":
			block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
		case "xtea":
			block, _ = kcp.NewXTEABlockCrypt(pass[:16])
		case "salsa20":
			block, _ = kcp.NewSalsa20BlockCrypt(pass)
		default:
			return nil, errors.New("[kcp] unknown crypt type '" + p.crypt + "'")
		}

		p.block = block
	}

	return p, nil
}

// NewKCPDialer returns a kcp proxy dialer
func NewKCPDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewKCP(s, dialer)
}

// NewKCPServer returns a kcp proxy server
func NewKCPServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	// TODO: check here
	if len(transport) < 2 {
		return nil, errors.New("[kcp] malformd listener:" + s)
	}

	p, err := NewKCP(transport[0], dialer)
	if err != nil {
		return nil, err
	}

	p.server, err = proxy.ServerFromURL(transport[1], dialer)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// ListenAndServe .
func (s *KCP) ListenAndServe() {
	l, err := kcp.ListenWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.F("[kcp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[kcp] listening on %s", s.addr)

	for {
		c, err := l.AcceptKCP()
		if err != nil {
			log.F("[kcp] failed to accept: %v", err)
			continue
		}

		// TODO: change them to customizable later?
		c.SetStreamMode(true)
		c.SetWriteDelay(false)
		c.SetNoDelay(0, 30, 2, 1)
		c.SetWindowSize(1024, 1024)
		c.SetMtu(1350)
		c.SetACKNoDelay(true)

		go s.Serve(c)
	}
}

// Serve serves requests
func (s *KCP) Serve(c net.Conn) {
	// we know the internal server will close the connection after serve
	// defer c.Close()

	if s.server != nil {
		s.server.Serve(c)
	}
}

// Addr returns forwarder's address
func (s *KCP) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *KCP) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy
func (s *KCP) Dial(network, addr string) (net.Conn, error) {
	// NOTE: kcp uses udp, we should dial remote server directly here
	c, err := kcp.DialWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	// TODO: change them to customizable later?
	c.SetStreamMode(true)
	c.SetWriteDelay(false)
	c.SetNoDelay(0, 30, 2, 1)
	c.SetWindowSize(1024, 1024)
	c.SetMtu(1350)
	c.SetACKNoDelay(true)

	c.SetDSCP(0)
	c.SetReadBuffer(4194304)
	c.SetWriteBuffer(4194304)

	return c, err
}

// DialUDP connects to the given address via the proxy
func (s *KCP) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("kcp client does not support udp now")
}
