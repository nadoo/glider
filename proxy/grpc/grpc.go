package grpc

import (
	"crypto/x509"
	"fmt"
	"github.com/nadoo/glider/proxy"
	"net"
	"net/url"
	"strings"
)

func init() {
	proxy.RegisterDialer("grpc", NewGRPCDialer)
}

// GRPC is the base gRPC proxy struct.
type GRPC struct {
	dialer      proxy.Dialer
	addr        string
	certPool    *x509.CertPool
	serviceName string
	serverName  string
	skipVerify  bool
	certFile    string
}

// NewGRPC returns a websocket proxy.
func NewGRPC(s string, d proxy.Dialer, p proxy.Proxy) (*GRPC, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse url err: %s", err)
	}

	addr := u.Host
	if addr == "" && d != nil {
		addr = d.Addr()
	}

	if _, p, _ := net.SplitHostPort(addr); p == "" {
		addr = net.JoinHostPort(addr, "443")
	}

	query := u.Query()
	g := &GRPC{
		dialer:      d,
		addr:        addr,
		skipVerify:  query.Get("skipVerify") == "true",
		serviceName: query.Get("serviceName"),
		serverName:  query.Get("serverName"),
		certFile:    query.Get("cert"),
	}

	if g.serviceName == "" {
		g.serviceName = "GunService"
	}

	if g.serverName == "" {
		g.serverName = g.addr[:strings.LastIndex(g.addr, ":")]
	}

	return g, nil
}

func init() {
	proxy.AddUsage("grpc", `
gRPC client scheme:
  grpc://host:port[?serviceName=SERVICENAME][&serverName=SERVERNAME][&skipVerify=true][&cert=PATH]
`)
}
