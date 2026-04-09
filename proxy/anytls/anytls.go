// Protocol spec:
// https://github.com/anytls/anytls-go/blob/main/docs/protocol.md

package anytls

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/anytls/session"
)

func init() {
	proxy.RegisterDialer("anytls", NewAnyTLSDialer)
	proxy.AddUsage("anytls", `
AnyTLS scheme:
  anytls://password@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&idleTimeout=SECONDS][&idleCheckInterval=SECONDS]
`)
}

// AnyTLS implements the anytls protocol client.
type AnyTLS struct {
	dialer     proxy.Dialer
	addr       string
	password   []byte // sha256 hash
	tlsConfig  *tls.Config
	serverName string
	skipVerify bool
	certFile   string

	idleCheckInterval time.Duration
	idleTimeout       time.Duration

	client *session.Client
}

// NewAnyTLSDialer returns a new anytls dialer.
func NewAnyTLSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("[anytls] parse url err: %s", err)
	}

	pass := u.User.Username()
	if pass == "" {
		return nil, errors.New("[anytls] password must be specified")
	}

	query := u.Query()

	a := &AnyTLS{
		dialer:     d,
		addr:       u.Host,
		skipVerify: query.Get("skipVerify") == "true",
		serverName: query.Get("serverName"),
		certFile:   query.Get("cert"),
	}

	// default port
	if a.addr != "" {
		if _, port, _ := net.SplitHostPort(a.addr); port == "" {
			a.addr = net.JoinHostPort(a.addr, "443")
		}
		if a.serverName == "" {
			a.serverName = a.addr[:strings.LastIndex(a.addr, ":")]
		}
	}

	// password sha256
	hash := sha256.Sum256([]byte(pass))
	a.password = hash[:]

	// idle session config
	a.idleCheckInterval = 30 * time.Second
	a.idleTimeout = 60 * time.Second
	if v := query.Get("idleCheckInterval"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			a.idleCheckInterval = time.Duration(n) * time.Second
		}
	}
	if v := query.Get("idleTimeout"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			a.idleTimeout = time.Duration(n) * time.Second
		}
	}

	// tls config
	a.tlsConfig = &tls.Config{
		ServerName:         a.serverName,
		InsecureSkipVerify: a.skipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if a.certFile != "" {
		certData, err := os.ReadFile(a.certFile)
		if err != nil {
			return nil, fmt.Errorf("[anytls] read cert file error: %s", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certData) {
			return nil, fmt.Errorf("[anytls] can not append cert file: %s", a.certFile)
		}
		a.tlsConfig.RootCAs = certPool
	}

	// session pool client
	a.client = session.NewClient(
		context.Background(),
		a.createOutboundConn,
		a.idleCheckInterval,
		a.idleTimeout,
	)

	return a, nil
}

// createOutboundConn dials to the server, performs TLS handshake, and sends authentication.
func (a *AnyTLS) createOutboundConn(ctx context.Context) (net.Conn, error) {
	rc, err := a.dialer.Dial("tcp", a.addr)
	if err != nil {
		return nil, fmt.Errorf("[anytls] dial to %s error: %s", a.addr, err)
	}

	tlsConn := tls.Client(rc, a.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		rc.Close()
		return nil, fmt.Errorf("[anytls] tls handshake error: %s", err)
	}

	// Send authentication: sha256(password) + padding_length(uint16) + padding
	var buf [34]byte // 32 bytes hash + 2 bytes padding length (0)
	copy(buf[:32], a.password)
	binary.BigEndian.PutUint16(buf[32:34], 0)

	if _, err := tlsConn.Write(buf[:]); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("[anytls] auth write error: %s", err)
	}

	return tlsConn, nil
}

// Addr returns the forwarder's address.
func (a *AnyTLS) Addr() string {
	if a.addr == "" {
		return a.dialer.Addr()
	}
	return a.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (a *AnyTLS) Dial(network, addr string) (net.Conn, error) {
	stream, err := a.client.CreateStream(context.Background())
	if err != nil {
		log.F("[anytls] create stream error: %s", err)
		return nil, err
	}

	// Write SOCKS address to indicate the destination
	target := socks.ParseAddr(addr)
	if target == nil {
		stream.Close()
		return nil, fmt.Errorf("[anytls] failed to parse target address: %s", addr)
	}

	if _, err := stream.Write(target); err != nil {
		stream.Close()
		return nil, fmt.Errorf("[anytls] failed to write target address: %s", err)
	}

	return stream, nil
}

// DialUDP connects to the given address via the proxy.
func (a *AnyTLS) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}
