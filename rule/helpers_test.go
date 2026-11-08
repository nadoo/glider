package rule

import (
	"errors"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
)

// Fake dialer schemes for deterministic cascade tests. testok always
// succeeds, testfail always fails and testflaky fails only its first dial.
// Every dialer counts its dials under its host name, so tests can assert how
// often a forwarder was dialed. Use unique host names per test.
const (
	testDialOK    = "testok"
	testDialFail  = "testfail"
	testDialFlaky = "testflaky"
)

var errTestDial = errors.New("test dial failed")

type testDialState struct {
	dials    atomic.Int64
	baseAddr atomic.Value // string
}

var testDialStates sync.Map // host -> *testDialState

func init() {
	proxy.RegisterDialer(testDialOK, newTestDialer)
	proxy.RegisterDialer(testDialFail, newTestDialer)
	proxy.RegisterDialer(testDialFlaky, newTestDialer)
}

func newTestDialer(s string, base proxy.Dialer) (proxy.Dialer, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	st := &testDialState{}
	if actual, loaded := testDialStates.LoadOrStore(u.Host, st); loaded {
		st = actual.(*testDialState)
	}
	st.baseAddr.Store(base.Addr())

	return &testDialer{addr: u.Host, mode: u.Scheme, state: st}, nil
}

type testDialer struct {
	addr  string
	mode  string
	state *testDialState
}

func (d *testDialer) Addr() string { return d.addr }

func (d *testDialer) failed(n int64) bool {
	switch d.mode {
	case testDialFail:
		return true
	case testDialFlaky:
		return n <= 1
	default:
		return false
	}
}

func (d *testDialer) Dial(network, addr string) (net.Conn, error) {
	if d.failed(d.state.dials.Add(1)) {
		return nil, errTestDial
	}
	server, client := net.Pipe()
	server.Close()
	return client, nil
}

func (d *testDialer) DialUDP(network, addr string) (net.PacketConn, error) {
	if d.failed(d.state.dials.Add(1)) {
		return nil, errTestDial
	}
	return net.ListenPacket("udp", "127.0.0.1:0")
}

func testDialCount(t *testing.T, host string) int64 {
	t.Helper()
	st, ok := testDialStates.Load(host)
	if !ok {
		t.Fatalf("no test dial state for host %q", host)
	}
	return st.(*testDialState).dials.Load()
}

// resetTestDialCount zeroes the dial counter for host. Call it after creating
// the forwarders and before dialing so repeated test runs stay independent.
func resetTestDialCount(t *testing.T, host string) {
	t.Helper()
	st, ok := testDialStates.Load(host)
	if !ok {
		t.Fatalf("no test dial state for host %q", host)
	}
	st.(*testDialState).dials.Store(0)
}

func testDialBase(t *testing.T, host string) string {
	t.Helper()
	st, ok := testDialStates.Load(host)
	if !ok {
		t.Fatalf("no test dial state for host %q", host)
	}
	addr, _ := st.(*testDialState).baseAddr.Load().(string)
	return addr
}

func newTestForwarder(t *testing.T, rawURL string) *Forwarder {
	t.Helper()
	f, err := ForwarderFromURL(rawURL, "", time.Second, 0)
	if err != nil {
		t.Fatalf("ForwarderFromURL(%q): %s", rawURL, err)
	}
	return f
}

// newTestCascade returns a cascade group with all forwarders enabled.
func newTestCascade(t *testing.T, strategy string, fwdrs ...*Forwarder) *CascadeGroup {
	t.Helper()
	return newTestCascadeWithAttempts(t, strategy, 0, fwdrs...)
}

// newTestCascadeWithAttempts sets the selection budget for each level.
func newTestCascadeWithAttempts(t *testing.T, strategy string, attempts int, fwdrs ...*Forwarder) *CascadeGroup {
	t.Helper()
	for _, f := range fwdrs {
		f.Enable()
	}
	c := &Strategy{Strategy: strategy, DialTimeout: 1}
	if attempts > 0 {
		c.Cascade = make(map[uint32]RetryOptions)
		for _, f := range fwdrs {
			c.Cascade[f.Cascade()] = RetryOptions{Attempts: &attempts}
		}
	}
	return NewCascadeGroup(fwdrs, c)
}

// newTestGroup returns a forwarder group with all forwarders enabled.
func newTestGroup(t *testing.T, strategy string, fwdrs ...*Forwarder) *FwdrGroup {
	t.Helper()
	for _, f := range fwdrs {
		f.Enable()
	}
	return newFwdrGroup("test", fwdrs, &Strategy{Strategy: strategy})
}

func testCascadeChain(t *testing.T, cg *CascadeGroup) proxy.Dialer {
	t.Helper()
	d, _, err := cg.chain(make([]cascadeSelection, len(cg.levels)), nil)
	if err != nil {
		return &errorDialer{err}
	}
	return d
}
