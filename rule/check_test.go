package rule

import (
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The file checker runs an external command, so the test helper is a shell
// script; Check reports the command's exit status.
func writeFileCheckerScript(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "check-helper.sh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileCheckerPassesOnSuccessfulCommand(t *testing.T) {
	path := writeFileCheckerScript(t, t.TempDir(), "#!/bin/sh\nexit 0\n")

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newFileChecker(path).Check(fwdr); err != nil {
		t.Fatalf("file checker error = %v, want success", err)
	}
}

func TestFileCheckerFailsOnFailingCommand(t *testing.T) {
	path := writeFileCheckerScript(t, t.TempDir(), "#!/bin/sh\nexit 1\n")

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newFileChecker(path).Check(fwdr); err == nil {
		t.Fatal("file checker accepted a failing command")
	}
}

func TestTcpCheckerReachesListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checker := newTcpChecker(ln.Addr().String(), time.Second)
	if _, err := checker.Check(fwdr); err != nil {
		t.Fatalf("tcp checker error = %v, want success", err)
	}

	// A host without port gets the default check port appended.
	defaulted := newTcpChecker("127.0.0.1", time.Second)
	if defaulted.addr != "127.0.0.1:80" {
		t.Fatalf("defaulted addr = %q, want 127.0.0.1:80", defaulted.addr)
	}
}

func TestTcpCheckerFailsOnClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newTcpChecker(addr, time.Second).Check(fwdr); err == nil {
		t.Fatal("tcp checker accepted a closed port")
	}
}

func TestHttpCheckerMatchesExpect(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checker := newHttpChecker(srv.Listener.Addr().String(), "/", "HTTP", time.Second, false)
	if _, err := checker.Check(fwdr); err != nil {
		t.Fatalf("http checker error = %v, want success", err)
	}
}

func TestHttpCheckerRejectsUnexpectedResponse(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()

	fwdr, err := DirectForwarder("", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checker := newHttpChecker(srv.Listener.Addr().String(), "/", "NEVER", time.Second, false)
	if _, err := checker.Check(fwdr); err == nil || !strings.Contains(err.Error(), "expect:") {
		t.Fatalf("error = %v, want expect mismatch", err)
	}
}
