// protocol spec:
// https://trojan-gfw.github.io/trojan/protocol

package trojan

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/proxy"
)

// Trojan is a base trojan struct.
type Trojan struct {
	dialer     proxy.Dialer
	proxy      proxy.Proxy
	addr       string
	pass       [56]byte
	withTLS    bool
	tlsConfig  *tls.Config
	serverName string
	skipVerify bool
	certFile   string
	keyFile    string
	fallback   string
}

// NewTrojan returns a trojan proxy.
func NewTrojan(s string, d proxy.Dialer, p proxy.Proxy) (*Trojan, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse url err: %s", err)
	}

	query := u.Query()
	t := &Trojan{
		dialer:     d,
		proxy:      p,
		addr:       u.Host,
		withTLS:    true,
		skipVerify: query.Get("skipVerify") == "true",
		serverName: query.Get("serverName"),
		certFile:   query.Get("cert"),
		keyFile:    query.Get("key"),
		fallback:   query.Get("fallback"),
	}

	if t.addr != "" {
		if _, port, _ := net.SplitHostPort(t.addr); port == "" {
			t.addr = net.JoinHostPort(t.addr, "443")
		}
		if t.serverName == "" {
			t.serverName = t.addr[:strings.LastIndex(t.addr, ":")]
		}
	}

	// pass
	pass := u.User.Username()
	if pass == "" {
		return nil, errors.New("[trojan] password must be specified")
	}

	hash := sha256.New224()
	hash.Write([]byte(pass))
	hex.Encode(t.pass[:], hash.Sum(nil))

	return t, nil
}

func init() {
	proxy.AddUsage("trojan", `
Trojan client scheme:
  trojan://pass@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH]
  trojanc://pass@host:port     (cleartext, without TLS)
  
Trojan server scheme:
  trojan://pass@host:port?cert=PATH&key=PATH[&fallback=127.0.0.1]
  trojanc://pass@host:port[?fallback=127.0.0.1]     (cleartext, without TLS)
`)
}
