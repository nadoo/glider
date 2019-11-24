package http

import (
	"bufio"
	"bytes"
	"errors"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/dongxinb/glider/common/log"
)

// Methods are http methods from rfc.
// https://www.ietf.org/rfc/rfc2616.txt, http methods must be uppercase
var Methods = [...][]byte{
	[]byte("GET"),
	[]byte("POST"),
	[]byte("PUT"),
	[]byte("DELETE"),
	[]byte("CONNECT"),
	[]byte("HEAD"),
	[]byte("OPTIONS"),
	[]byte("TRACE"),
	[]byte("PATCH"),
}

type request struct {
	method string
	uri    string
	proto  string
	auth   string
	header textproto.MIMEHeader

	target string // target host with port
	ruri   string // relative uri
	absuri string // absolute uri
}

func parseRequest(r *bufio.Reader) (*request, error) {
	tpr := textproto.NewReader(r)
	line, err := tpr.ReadLine()
	if err != nil {
		return nil, err
	}

	method, uri, proto, ok := parseStartLine(line)
	if !ok {
		return nil, errors.New("error in parseStartLine")
	}

	header, err := tpr.ReadMIMEHeader()
	if err != nil {
		log.F("[http] read header error:%s", err)
		return nil, err
	}

	auth := header.Get("Proxy-Authorization")

	cleanHeaders(header)
	header.Set("Connection", "close")

	u, err := url.ParseRequestURI(uri)
	if err != nil {
		log.F("[http] parse request url error: %s, uri: %s", err, uri)
		return nil, err
	}

	var tgt = u.Host
	if !strings.Contains(u.Host, ":") {
		tgt += ":80"
	}

	req := &request{
		method: method,
		uri:    uri,
		proto:  proto,
		auth:   auth,
		header: header,
		target: tgt,
	}

	if u.IsAbs() {
		req.absuri = u.String()
		u.Scheme = ""
		u.Host = ""
		req.ruri = u.String()
	} else {
		req.ruri = u.String()

		base, err := url.Parse("http://" + header.Get("Host"))
		if err != nil {
			return nil, err
		}
		u = base.ResolveReference(u)
		req.absuri = u.String()
	}

	return req, nil
}

func (r *request) Marshal() []byte {
	var buf bytes.Buffer
	writeStartLine(&buf, r.method, r.ruri, r.proto)
	writeHeaders(&buf, r.header)
	return buf.Bytes()
}

func (r *request) MarshalAbs() []byte {
	var buf bytes.Buffer
	writeStartLine(&buf, r.method, r.absuri, r.proto)
	writeHeaders(&buf, r.header)
	return buf.Bytes()
}
