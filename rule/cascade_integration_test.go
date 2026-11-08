package rule

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nadoo/glider/proxy/http"
	"github.com/nadoo/glider/proxy/trojan"
)

func connectProxy(t *testing.T, allowed string) (string, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	var wg sync.WaitGroup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodConnect || r.Host != allowed {
			http.Error(w, "destination not allowed", http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", allowed, time.Second)
		if err != nil {
			http.Error(w, "connect failed", http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		client, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		wg.Add(1)
		defer wg.Done()
		defer client.Close()
		client.SetDeadline(time.Now().Add(3 * time.Second))
		upstream.SetDeadline(time.Now().Add(3 * time.Second))
		io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() { io.Copy(upstream, buf); upstream.Close(); close(done) }()
		io.Copy(client, upstream)
		client.Close()
		<-done
	}))
	t.Cleanup(func() { server.Close(); wg.Wait() })
	return strings.TrimPrefix(server.URL, "http://"), &calls
}

func TestCascadeConnectsOnlyCompleteChain(t *testing.T) {
	var accepted atomic.Int64
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "cascade payload")
	}))
	target.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			accepted.Add(1)
		}
	}
	target.Start()
	t.Cleanup(target.Close)
	addr := strings.TrimPrefix(target.URL, "http://")
	second, secondCalls := connectProxy(t, addr)
	first, firstCalls := connectProxy(t, second)
	p := NewProxy([]string{
		"http://" + first + "#cascade=1",
		"http://" + second + "#cascade=2",
	}, &Strategy{Strategy: "prs", Check: "disable", DialTimeout: 1}, nil)
	p.Check()
	for _, adapter := range []bool{false, true} {
		var c net.Conn
		var err error
		if adapter {
			c, err = p.NextDialer(addr).Dial("tcp", addr)
		} else {
			c, _, err = p.Dial("tcp", addr)
		}
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err = io.WriteString(c, "GET / HTTP/1.1\r\nHost: target\r\nConnection: close\r\n\r\n"); err != nil {
			c.Close()
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			c.Close()
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		c.Close()
		if err != nil || string(body) != "cascade payload" {
			t.Fatalf("body=%q error=%v", body, err)
		}
	}
	if accepted.Load() != 2 || firstCalls.Load() != 2 || secondCalls.Load() != 2 {
		t.Fatalf("connections: target=%d first=%d second=%d, want two each", accepted.Load(), firstCalls.Load(), secondCalls.Load())
	}
}

func TestCascadeClosesRejectedHTTPConnections(t *testing.T) {
	closed := make(chan struct{}, 3)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rejected", http.StatusForbidden)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	f := newTestForwarder(t, server.URL+"#cascade=1")
	cg := newTestCascadeWithAttempts(t, "prs", 3, f)
	if c, _, err := cg.Dial("tcp", "target:80"); err == nil {
		c.Close()
		t.Fatal("accepted rejected connection")
	}
	for range 3 {
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("failed cascade attempt left a connection open")
		}
	}
}

func TestCascadeDeliversUDPThroughHTTPAndTrojan(t *testing.T) {
	target, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	target.SetDeadline(time.Now().Add(3 * time.Second))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	server, err := trojan.NewClearTextServer("trojanc://test@"+ln.Addr().String(), NewProxy(nil, &Strategy{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		server.Serve(c)
	}()
	first, calls := connectProxy(t, ln.Addr().String())
	p := NewProxy([]string{
		"http://" + first + "#cascade=1",
		"trojanc://test@" + ln.Addr().String() + "#cascade=2",
	}, &Strategy{Strategy: "prs", Check: "disable", DialTimeout: 1}, nil)
	p.Check()
	pc, _, err := p.DialUDP("udp", target.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	pc.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := pc.WriteTo([]byte("request"), target.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, from, err := target.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "request" {
		t.Fatalf("request=%q error=%v", buf[:n], err)
	}
	if _, err := target.WriteTo([]byte("reply"), from); err != nil {
		t.Fatal(err)
	}
	n, _, err = pc.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "reply" {
		t.Fatalf("reply=%q error=%v", buf[:n], err)
	}
	pc.Close()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatal("Trojan server did not exit")
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP connections=%d, want 1", calls.Load())
	}
}
