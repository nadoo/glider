package ws

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

func init() {
	proxy.RegisterDialer("ws", NewWSDialer)
	proxy.RegisterServer("ws", NewWSServer)
	proxy.RegisterDialer("wss", NewWSSDialer)
	proxy.RegisterServer("wss", NewWSSServer)
}

// WS is the base ws proxy struct.
type WS struct {
	dialer     proxy.Dialer
	proxy      proxy.Proxy
	addr       string
	host       string
	path       string
	origin     string
	withTLS    bool
	tlsConfig  *tls.Config
	serverName string
	skipVerify bool
	certFile   string
	keyFile    string
	server     proxy.Server
}

// NewWS returns a websocket proxy.
func NewWS(s string, d proxy.Dialer, p proxy.Proxy, withTLS bool) (*WS, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse url err: %s", err)
	}

	addr := u.Host
	if addr == "" && d != nil {
		addr = d.Addr()
	}

	if _, p, _ := net.SplitHostPort(addr); p == "" {
		if withTLS {
			addr = net.JoinHostPort(addr, "443")
		} else {
			addr = net.JoinHostPort(addr, "80")
		}
	}

	query := u.Query()
	w := &WS{
		dialer:     d,
		proxy:      p,
		addr:       addr,
		path:       u.Path,
		host:       query.Get("host"),
		origin:     query.Get("origin"),
		withTLS:    withTLS,
		skipVerify: query.Get("skipVerify") == "true",
		serverName: query.Get("serverName"),
		certFile:   query.Get("cert"),
		keyFile:    query.Get("key"),
	}

	if w.host == "" {
		w.host = w.addr
	}

	if w.path == "" {
		w.path = "/"
	}

	if w.serverName == "" {
		w.serverName = w.addr[:strings.LastIndex(w.addr, ":")]
	}

	return w, nil
}

// parseFirstLine parses "GET /foo HTTP/1.1" OR "HTTP/1.1 200 OK" into its three parts.
// TODO: move to separate http lib package for reuse(also for http proxy module)
func parseFirstLine(line string) (r1, r2, r3 string, ok bool) {
	s1 := strings.Index(line, " ")
	s2 := strings.Index(line[s1+1:], " ")
	if s1 < 0 || s2 < 0 {
		return
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func generateClientKey() string {
	p := pool.GetBuffer(16)
	defer pool.PutBuffer(p)
	rand.Read(p)
	return base64.StdEncoding.EncodeToString(p)
}

func computeServerKey(clientKey string) string {
	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write(keyGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func init() {
	proxy.AddUsage("ws", `
Websocket client scheme:
  ws://host:port[/path][?host=HOST][&origin=ORIGIN]
  wss://host:port[/path][?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&host=HOST][&origin=ORIGIN]
  
Websocket server scheme:
  ws://:port[/path][?host=HOST]
  wss://:port[/path]?cert=PATH&key=PATH[?host=HOST]
  
Websocket with a specified proxy protocol:
  ws://host:port[/path][?host=HOST],scheme://
  ws://host:port[/path][?host=HOST],http://[user:pass@]
  ws://host:port[/path][?host=HOST],socks5://[user:pass@]
  
TLS and Websocket with a specified proxy protocol:
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],ws://[@/path[?host=HOST]],scheme://
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],http://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],vmess://[security:]uuid@?alterID=num
`)
}
