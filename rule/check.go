package rule

import (
	"bytes"
	"io"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
)

// Checker is a forwarder health checker.
type Checker interface {
	Check(fwdr *Forwarder) (healthy bool)
}

type tcpChecker struct {
	addr    string
	timeout time.Duration
}

func newTcpChecker(addr string, timeout time.Duration) *tcpChecker {
	return &tcpChecker{addr, timeout}
}

func (c *tcpChecker) Check(fwdr *Forwarder) bool {
	startTime := time.Now()

	rc, err := fwdr.Dial("tcp", c.addr)
	if err != nil {
		log.F("[check] tcp:%s(%d), FAILED. error in dial: %s", fwdr.Addr(), fwdr.Priority(), err)
		fwdr.Disable()
		return false
	}
	defer rc.Close()

	if c.timeout > 0 {
		rc.SetDeadline(time.Now().Add(c.timeout))
	}

	elapsed := time.Since(startTime)
	fwdr.SetLatency(int64(elapsed))

	if elapsed > c.timeout {
		log.F("[check] tcp:%s(%d), FAILED. check timeout: %s", fwdr.Addr(), fwdr.Priority(), elapsed)
		fwdr.Disable()
		return false
	}

	log.F("[check] tcp:%s(%d), SUCCESS. elapsed: %s", fwdr.Addr(), fwdr.Priority(), elapsed)
	fwdr.Enable()

	return true
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

func (c *httpChecker) Check(fwdr *Forwarder) bool {
	startTime := time.Now()
	rc, err := fwdr.Dial("tcp", c.addr)
	if err != nil {
		log.F("[check] %s(%d) -> http://%s, FAILED. error in dial: %s", fwdr.Addr(), fwdr.Priority(), c.addr, err)
		fwdr.Disable()
		return false
	}
	defer rc.Close()

	if c.timeout > 0 {
		rc.SetDeadline(time.Now().Add(c.timeout))
	}

	_, err = io.WriteString(rc, "GET "+c.uri+" HTTP/1.1\r\nHost:"+c.addr+"\r\nConnection: close"+"\r\n\r\n")
	if err != nil {
		log.F("[check] %s(%d) -> http://%s, FAILED. error in write: %s", fwdr.Addr(), fwdr.Priority(), c.addr, err)
		fwdr.Disable()
		return false
	}

	r := pool.GetBufReader(rc)
	defer pool.PutBufReader(r)

	line, _, err := r.ReadLine()
	if err != nil {
		log.F("[check] %s(%d) -> http://%s, FAILED. error in read: %s", fwdr.Addr(), fwdr.Priority(), c.addr, err)
		fwdr.Disable()
		return false
	}

	if !bytes.Contains(line, []byte(c.expect)) {
		log.F("[check] %s(%d) -> http://%s, FAILED. expect: %s, server response: %s", fwdr.Addr(), fwdr.Priority(), c.addr, c.expect, line)
		fwdr.Disable()
		return false
	}

	elapsed := time.Since(startTime)
	fwdr.SetLatency(int64(elapsed))

	if elapsed > c.timeout {
		log.F("[check] %s(%d) -> http://%s, FAILED. check timeout: %s", fwdr.Addr(), fwdr.Priority(), c.addr, elapsed)
		fwdr.Disable()
		return false
	}

	log.F("[check] %s(%d) -> http://%s, SUCCESS. elapsed: %s", fwdr.Addr(), fwdr.Priority(), c.addr, elapsed)
	fwdr.Enable()

	return true
}
