package ws

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

// WS is the base ws proxy struct.
type WS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string
	host   string
	path   string
	origin string

	server proxy.Server
}

// NewWS returns a websocket proxy.
func NewWS(s string, d proxy.Dialer, p proxy.Proxy) (*WS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[ws] parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	// TODO:
	if addr == "" {
		addr = d.Addr()
	}

	w := &WS{
		dialer: d,
		proxy:  p,
		addr:   addr,
		host:   u.Query().Get("host"),
		path:   u.Path,
		origin: u.Query().Get("origin"),
	}

	if w.host == "" {
		w.host = addr
	}

	if w.path == "" {
		w.path = "/"
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
