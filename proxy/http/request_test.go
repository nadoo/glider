package http

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseRequestTarget(t *testing.T) {
	for _, tt := range []struct {
		name   string
		uri    string
		host   string
		target string
	}{
		{"absolute hostname", "http://example.com/path", "other.example", "example.com:80"},
		{"absolute hostname port", "http://example.com:8080/path", "other.example", "example.com:8080"},
		{"absolute IPv6", "http://[2001:db8::1]/path", "other.example", "[2001:db8::1]:80"},
		{"absolute IPv6 port", "http://[2001:db8::1]:8080/path", "other.example", "[2001:db8::1]:8080"},
		{"origin hostname", "/path", "example.com", "example.com:80"},
		{"origin hostname port", "/path", "example.com:8080", "example.com:8080"},
		{"origin IPv6", "/path", "[2001:db8::1]", "[2001:db8::1]:80"},
		{"origin IPv6 port", "/path", "[2001:db8::1]:8080", "[2001:db8::1]:8080"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := "GET " + tt.uri + " HTTP/1.1\r\nHost: " + tt.host + "\r\n\r\n"
			req, err := parseRequest(bufio.NewReader(strings.NewReader(raw)))
			if err != nil {
				t.Fatal(err)
			}
			if req.target != tt.target {
				t.Errorf("target = %q, want %q", req.target, tt.target)
			}
			if req.ruri != "/path" {
				t.Errorf("relative URI = %q, want /path", req.ruri)
			}
		})
	}
}

func TestParseConnectAuthority(t *testing.T) {
	for _, authority := range []string{"example.com:443", "[2001:db8::1]:8443"} {
		t.Run(authority, func(t *testing.T) {
			raw := "CONNECT " + authority + " HTTP/1.1\r\nHost: other.example:8080\r\n\r\n"
			req, err := parseRequest(bufio.NewReader(strings.NewReader(raw)))
			if err != nil {
				t.Fatal(err)
			}
			if req.uri != authority || req.target != authority {
				t.Errorf("CONNECT uri=%q target=%q, want %q", req.uri, req.target, authority)
			}
		})
	}
}
