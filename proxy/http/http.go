// https://developer.mozilla.org/en-US/docs/Web/HTTP/Messages
// NOTE: never keep-alive so the implementation can be much easier.

// Package http implements a http proxy.
package http

import (
	"encoding/base64"
	"io"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// HTTP struct.
type HTTP struct {
	dialer   proxy.Dialer
	proxy    proxy.Proxy
	addr     string
	user     string
	password string
	pretend  bool
}

func init() {
	proxy.RegisterDialer("http", NewHTTPDialer)
	proxy.RegisterServer("http", NewHTTPServer)
}

// NewHTTP returns a http proxy.
func NewHTTP(s string, d proxy.Dialer, p proxy.Proxy) (*HTTP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()

	h := &HTTP{
		dialer:   d,
		proxy:    p,
		addr:     addr,
		user:     user,
		password: pass,
		pretend:  false,
	}

	if u.Query().Get("pretend") == "true" {
		h.pretend = true
	}

	return h, nil
}

// parseStartLine parses "GET /foo HTTP/1.1" OR "HTTP/1.1 200 OK" into its three parts.
func parseStartLine(line string) (r1, r2, r3 string, ok bool) {
	s1 := strings.Index(line, " ")
	s2 := strings.Index(line[s1+1:], " ")
	if s1 < 0 || s2 < 0 {
		return
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func cleanHeaders(header textproto.MIMEHeader) {
	header.Del("Proxy-Connection")
	header.Del("Connection")
	header.Del("Keep-Alive")
	header.Del("Proxy-Authenticate")
	header.Del("Proxy-Authorization")
	header.Del("TE")
	header.Del("Trailers")
	header.Del("Transfer-Encoding")
	header.Del("Upgrade")
}

func writeStartLine(w io.Writer, s1, s2, s3 string) {
	io.WriteString(w, s1+" "+s2+" "+s3+"\r\n")
}

func writeHeaders(w io.Writer, header textproto.MIMEHeader) {
	for key, values := range header {
		for _, v := range values {
			io.WriteString(w, key+": "+v+"\r\n")
		}
	}
	io.WriteString(w, "\r\n")
}

func extractUserPass(auth string) (username, password string, ok bool) {
	if !strings.HasPrefix(auth, "Basic ") {
		return
	}

	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return
	}

	s := string(b)
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return
	}

	return s[:idx], s[idx+1:], true
}

func init() {
	proxy.AddUsage("http", `
Http scheme:
  http://[user:pass@]host:port
`)
}
