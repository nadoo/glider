package rule

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

// Checker is a forwarder health checker.
type Checker interface {
	Check(dialer proxy.Dialer) (elap time.Duration, err error)
}

type tcpChecker struct {
	addr    string
	timeout time.Duration
}

func newTcpChecker(addr string, timeout time.Duration) *tcpChecker {
	return &tcpChecker{addr, timeout}
}

// Check implements the Checker interface.
func (c *tcpChecker) Check(dialer proxy.Dialer) (time.Duration, error) {
	startTime := time.Now()
	rc, err := dialer.Dial("tcp", c.addr)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	return time.Since(startTime), nil
}

type httpChecker struct {
	addr    string
	uri     string
	expect  string
	timeout time.Duration
}

func newHttpChecker(addr, uri, expect string, timeout time.Duration) *httpChecker {
	return &httpChecker{addr, uri, expect, timeout}
}

// Check implements the Checker interface.
func (c *httpChecker) Check(dialer proxy.Dialer) (time.Duration, error) {
	startTime := time.Now()
	rc, err := dialer.Dial("tcp", c.addr)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	if c.timeout > 0 {
		rc.SetDeadline(time.Now().Add(c.timeout))
	}

	if _, err = io.WriteString(rc,
		"GET "+c.uri+" HTTP/1.1\r\nHost:"+c.addr+"\r\nConnection: close"+"\r\n\r\n"); err != nil {
		return 0, err
	}

	r := pool.GetBufReader(rc)
	defer pool.PutBufReader(r)

	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}

	if !strings.Contains(line, c.expect) {
		return 0, fmt.Errorf("expect: %s, got: %s", c.expect, line)
	}

	elapsed := time.Since(startTime)
	if elapsed > c.timeout {
		return elapsed, errors.New("timeout")
	}

	return elapsed, nil
}

type fileChecker struct{ path string }

func newFileChecker(path string) *fileChecker { return &fileChecker{path} }

// Check implements the Checker interface.
func (c *fileChecker) Check(dialer proxy.Dialer) (time.Duration, error) {
	cmd := exec.Command(c.path)
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "FORWARDER_ADDR="+dialer.Addr())
	return 0, cmd.Run()
}
