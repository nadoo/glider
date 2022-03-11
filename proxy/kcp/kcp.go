package kcp

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	kcp "github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// KCP struct.
type KCP struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	key   string
	crypt string
	block kcp.BlockCrypt
	mode  string

	dataShards   int
	parityShards int

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("kcp", NewKCPDialer)
	proxy.RegisterServer("kcp", NewKCPServer)
}

// NewKCP returns a kcp proxy struct.
func NewKCP(s string, d proxy.Dialer, p proxy.Proxy) (*KCP, error) {
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

	k := &KCP{
		dialer:       d,
		proxy:        p,
		addr:         addr,
		key:          key,
		crypt:        crypt,
		mode:         query.Get("mode"),
		dataShards:   int(dataShards),
		parityShards: int(parityShards),
	}

	if k.crypt != "" {
		k.block, err = block(k.crypt, k.key)
		if err != nil {
			return nil, fmt.Errorf("[kcp] error: %s", err)
		}
	}

	if k.mode == "" {
		k.mode = "fast"
	}

	return k, nil
}

func block(crypt, key string) (block kcp.BlockCrypt, err error) {
	pass := pbkdf2.Key([]byte(key), []byte("kcp-go"), 4096, 32, sha1.New)
	switch crypt {
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
		err = errors.New("unknown crypt type '" + crypt + "'")
	}
	return block, err
}

// NewKCPDialer returns a kcp proxy dialer.
func NewKCPDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewKCP(s, d, nil)
}

// NewKCPServer returns a kcp proxy server.
func NewKCPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	k, err := NewKCP(schemes[0], nil, p)
	if err != nil {
		return nil, err
	}

	if len(schemes) > 1 {
		k.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return k, nil
}

// ListenAndServe listens on server's addr and serves connections.
func (s *KCP) ListenAndServe() {
	l, err := kcp.ListenWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.Fatalf("[kcp] failed to listen on %s: %v", s.addr, err)
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

		s.setParams(c)

		go s.Serve(c)
	}
}

// Serve serves connections.
func (s *KCP) Serve(c net.Conn) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[kcp] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}

	defer rc.Close()

	log.F("[kcp] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[kcp] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// Addr returns forwarder's address.
func (s *KCP) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *KCP) Dial(network, addr string) (net.Conn, error) {
	// NOTE: kcp uses udp, we should dial remote server directly here
	c, err := kcp.DialWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.F("[kcp] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	s.setParams(c)

	c.SetDSCP(0)
	c.SetReadBuffer(4194304)
	c.SetWriteBuffer(4194304)

	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *KCP) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func (s *KCP) setParams(c *kcp.UDPSession) {
	// TODO: change them to customizable later?
	c.SetStreamMode(true)
	c.SetWriteDelay(false)

	switch s.mode {
	case "normal":
		c.SetNoDelay(0, 40, 2, 1)
	case "fast":
		c.SetNoDelay(0, 30, 2, 1)
	case "fast2":
		c.SetNoDelay(1, 20, 2, 1)
	case "fast3":
		c.SetNoDelay(1, 10, 2, 1)
	default:
		log.F("[kcp] unkonw mode: %s, use fast mode instead", s.mode)
		c.SetNoDelay(0, 30, 2, 1)
	}

	c.SetWindowSize(1024, 1024)
	c.SetMtu(1350)
	c.SetACKNoDelay(true)
}

func init() {
	proxy.AddUsage("kcp", `
KCP scheme:
  kcp://CRYPT:KEY@host:port[?dataShards=NUM&parityShards=NUM&mode=MODE]
  
Available crypt types for KCP:
  none, sm4, tea, xor, aes, aes-128, aes-192, blowfish, twofish, cast5, 3des, xtea, salsa20
  
Available modes for KCP:
  fast, fast2, fast3, normal, default: fast
`)
}
