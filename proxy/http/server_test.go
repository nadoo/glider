package http

import (
	"bufio"
	"bytes"
	"io"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
)

type directTestProxy struct{}

func (directTestProxy) Addr() string { return "direct" }
func (p directTestProxy) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	c, err := net.Dial(network, addr)
	return c, &proxy.Direct{}, err
}
func (directTestProxy) DialUDP(string, string) (net.PacketConn, proxy.UDPDialer, error) {
	return nil, &proxy.Direct{}, proxy.ErrNotSupported
}
func (directTestProxy) NextDialer(string) proxy.Dialer { return &proxy.Direct{} }
func (directTestProxy) Record(proxy.Dialer, bool)      {}

func TestServeForwardsChunkedRequestBody(t *testing.T) {
	origin := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			stdhttp.Error(w, err.Error(), stdhttp.StatusBadRequest)
			return
		}
		w.Write(body)
	}))
	defer origin.Close()
	client, server := net.Pipe()
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() {
		defer close(done)
		s := &HTTP{proxy: directTestProxy{}}
		s.Serve(server)
	}()
	request := "POST " + origin.URL + "/upload HTTP/1.1\r\n" +
		"Host: " + strings.TrimPrefix(origin.URL, "http://") + "\r\n" +
		"Transfer-Encoding: chunked\r\nConnection: close\r\n\r\n" +
		"5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n"
	if _, err := io.WriteString(client, request); err != nil {
		t.Fatal(err)
	}
	response, err := stdhttp.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusOK || string(body) != "hello world" {
		t.Fatalf("upstream response = %d %q, want 200 hello world", response.StatusCode, body)
	}
	client.Close()
	<-done
}

func TestRequestFramingHeaders(t *testing.T) {
	for _, tt := range []struct {
		name           string
		headers        string
		transferCoding bool
		contentLength  bool
	}{
		{"chunked", "Transfer-Encoding: chunked\r\n", true, false},
		{"chunked with length", "Transfer-Encoding: chunked\r\nContent-Length: 1\r\n", true, false},
		{"content length", "Content-Length: 11\r\n", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := "POST http://example.com/upload HTTP/1.1\r\nHost: example.com\r\n" + tt.headers + "\r\n"
			req, err := parseRequest(bufio.NewReader(strings.NewReader(raw)))
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			req.WriteBuf(&wire)
			if got := strings.Contains(wire.String(), "Transfer-Encoding:"); got != tt.transferCoding {
				t.Errorf("Transfer-Encoding present = %v, want %v", got, tt.transferCoding)
			}
			if got := strings.Contains(wire.String(), "Content-Length:"); got != tt.contentLength {
				t.Errorf("Content-Length present = %v, want %v", got, tt.contentLength)
			}
		})
	}
}
