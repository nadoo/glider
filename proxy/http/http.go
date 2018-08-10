// http proxy
// NOTE: never keep-alive so the implementation can be much easier.

package http

import (
	"bytes"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
)

// HTTP struct
type HTTP struct {
	addr     string
	user     string
	password string
}

// NewHTTP returns a http base struct
func NewHTTP(s string) (*HTTP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()

	h := &HTTP{
		addr:     addr,
		user:     user,
		password: pass,
	}

	return h, nil
}

// parseFirstLine parses "GET /foo HTTP/1.1" OR "HTTP/1.1 200 OK" into its three parts.
func parseFirstLine(tp *textproto.Reader) (r1, r2, r3 string, ok bool) {
	line, err := tp.ReadLine()
	// log.F("first line: %s", line)
	if err != nil {
		log.F("[http] read first line error: %s", err)
		return
	}

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

func writeFirstLine(s1, s2, s3 string, buf *bytes.Buffer) {
	buf.Write([]byte(s1))
	buf.Write([]byte(" "))
	buf.Write([]byte(s2))
	buf.Write([]byte(" "))
	buf.Write([]byte(s3))
	buf.Write([]byte("\r\n"))
}

func writeHeaders(header textproto.MIMEHeader, buf *bytes.Buffer) {
	for key, values := range header {
		buf.Write([]byte(key))
		buf.Write([]byte(": "))
		for k, v := range values {
			buf.Write([]byte(v))
			if k > 0 {
				buf.Write([]byte(" "))
			}
		}
		buf.Write([]byte("\r\n"))
	}

	//header ended
	buf.Write([]byte("\r\n"))
}
