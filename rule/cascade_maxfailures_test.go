package rule

import (
	"errors"
	"testing"
)

// TestCascadeDialFailuresDoNotDisableHops guards the health-check rule:
// chain dials go through the raw dialer stack, so a failed dial never counts
// as a forwarder failure and maxfailures cannot disable a hop this way.
func TestCascadeDialFailuresDoNotDisableHops(t *testing.T) {
	a := newTestForwarder(t, "testfail://maxfail-a#cascade=1")
	a.SetMaxFailures(1)

	cg := newTestCascadeWithAttempts(t, "prs", 2, a)
	resetTestDialCount(t, "maxfail-a")
	if _, _, err := cg.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
		t.Fatalf("dial error = %v, want %v", err, errTestDial)
	}

	if !a.Enabled() {
		t.Fatal("dial failure disabled the hop, want health checks to be the only disabler")
	}
	if n := testDialCount(t, "maxfail-a"); n != 2 {
		t.Fatalf("maxfail-a dials = %d, want one dial per attempt", n)
	}
}

// TestCascadeSkipsDisabledHop proves that a hop disabled by a health check
// drops out of the chain within the same request and later levels still get
// dialed.
func TestCascadeSkipsDisabledHop(t *testing.T) {
	a := newTestForwarder(t, "testfail://skip-fail#cascade=1")
	b := newTestForwarder(t, "testok://skip-ok#cascade=2")

	cg := newTestCascadeWithAttempts(t, "pr", 2, a, b)
	a.Disable() // as a health check would do after reaching maxfailures
	resetTestDialCount(t, "skip-ok")

	c, err := cg.NextDialer("target:80").Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("dial error = %v, want fallback chain through level 2", err)
	}
	c.Close()

	if n := testDialCount(t, "skip-fail"); n != 0 {
		t.Fatalf("skip-fail dials = %d, want 0 (hop is disabled)", n)
	}
	if n := testDialCount(t, "skip-ok"); n != 1 {
		t.Fatalf("skip-ok dials = %d, want 1", n)
	}
}

// TestCascadeDialUDPDirectFallback proves that pr keeps its direct fallback
// for UDP requests after the chain budget is exhausted.
func TestCascadeDialUDPDirectFallback(t *testing.T) {
	f := newTestForwarder(t, "testfail://udp-fallback#cascade=1")
	cg := newTestCascadeWithAttempts(t, "pr", 1, f)

	conn, _, err := cg.DialUDP("udp", "target:53")
	if err != nil {
		t.Fatalf("DialUDP error = %v, want direct fallback", err)
	}
	conn.Close()
}

// TestCascadeDialUDPStrictBudgetError proves that prs reports the dial error
// instead of falling back to direct UDP.
func TestCascadeDialUDPStrictBudgetError(t *testing.T) {
	f := newTestForwarder(t, "testfail://udp-strict#cascade=1")
	cg := newTestCascadeWithAttempts(t, "prs", 1, f)

	_, _, err := cg.DialUDP("udp", "target:53")
	if !errors.Is(err, errTestDial) {
		t.Fatalf("DialUDP error = %v, want %v", err, errTestDial)
	}
}

// TestCascadeAdapterAddr proves the adapter keeps a non-empty address so
// NextDialer callers can log a route even before dialing.
func TestCascadeAdapterAddr(t *testing.T) {
	f := newTestForwarder(t, "testok://adapter-addr#cascade=1")
	cg := newTestCascade(t, "pr", f)

	if addr := cg.NextDialer("target:80").Addr(); addr != "cascade" {
		t.Fatalf("adapter addr = %q, want cascade", addr)
	}
}
