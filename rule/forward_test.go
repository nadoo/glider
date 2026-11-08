package rule

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestForwarderMaxFailuresDisables proves the maxfailures mechanism used by
// health checks: reaching the limit disables the forwarder.
func TestForwarderMaxFailuresDisables(t *testing.T) {
	f := newTestForwarder(t, "testok://maxfailures-unit")
	f.SetMaxFailures(1)
	f.Enable()

	f.IncFailures()
	if f.Enabled() {
		t.Fatal("forwarder still enabled after reaching maxfailures")
	}

	f.Enable()
	if !f.Enabled() {
		t.Fatal("re-enable after maxfailures failed")
	}
}

// TestForwarderDialCountsFailures proves that relayed dial errors through
// Forwarder.Dial count toward failures (plain groups and health checks).
func TestForwarderDialCountsFailures(t *testing.T) {
	f := newTestForwarder(t, "testfail://forwarder-dial")
	f.Enable()

	if _, err := f.Dial("tcp", "target:80"); err == nil {
		t.Fatal("dial expected to fail")
	}
	if f.Failures() != 1 {
		t.Fatalf("failures = %d, want 1", f.Failures())
	}
	if !f.Enabled() {
		t.Fatal("forwarder disabled without a maxfailures limit")
	}
}
func TestParseOptionPriority(t *testing.T) {
	f := &Forwarder{}
	if err := f.parseOption("priority=99"); err != nil {
		t.Fatalf("parseOption error: %s", err)
	}
	if p := f.Priority(); p != 99 {
		t.Errorf("priority: got %d, want 99", p)
	}
}

func TestParseOptionWeight(t *testing.T) {
	f := &Forwarder{}
	if err := f.parseOption("weight=42"); err != nil {
		t.Fatalf("parseOption error: %s", err)
	}
	if w := f.Weight(); w != 42 {
		t.Errorf("weight: got %d, want 42", w)
	}
}

func TestSetWeightAllowsExplicitZero(t *testing.T) {
	f := &Forwarder{}
	f.SetWeight(0)
	if w := f.Weight(); w != 0 {
		t.Errorf("weight: got %d, want explicit zero", w)
	}
}

func TestParseOptionCascade(t *testing.T) {
	f := &Forwarder{}
	if err := f.parseOption("cascade=3"); err != nil {
		t.Fatalf("parseOption error: %s", err)
	}
	if c := f.Cascade(); c != 3 {
		t.Errorf("cascade: got %d, want 3", c)
	}
}

func TestForwarderRetryOptions(t *testing.T) {
	f := newTestForwarder(t, "testok://retry-options#retries=2&retrydelay=100")
	if f.retries != 2 || f.retryDelay != 100*time.Millisecond {
		t.Fatalf("retries=%d delay=%v", f.retries, f.retryDelay)
	}
	f = newTestForwarder(t, "testok://retry-defaults")
	if f.retries != 0 || f.retryDelay != 200*time.Millisecond {
		t.Fatalf("retries=%d delay=%v", f.retries, f.retryDelay)
	}
}

func TestParseOptionEmptyValuesAreIgnored(t *testing.T) {
	f := &Forwarder{}
	if err := f.parseOption("priority=&weight=&cascade="); err != nil {
		t.Fatal(err)
	}
	if f.Priority() != 0 || f.Weight() != 1 || f.Cascade() != 0 {
		t.Fatal("empty options changed defaults")
	}
}

func TestForwarderLatency(t *testing.T) {
	f := &Forwarder{}
	f.SetLatency(int64(time.Second))
	if f.Latency() != int64(time.Second) {
		t.Fatal("latency did not round trip")
	}
}

func TestForwarderRejectsMultipleOptionSeparators(t *testing.T) {
	_, err := ForwarderFromURL("testok://test-user:test-password@multi#weight=1#cascade=1", "", time.Second, 0)
	if err == nil {
		t.Fatal("expected error for a second '#' in forwarder URL")
	}
	if strings.Contains(err.Error(), "test-user") || strings.Contains(err.Error(), "test-password") {
		t.Fatal("forwarder error contains credentials")
	}
}

func TestParseOptionAll(t *testing.T) {
	f := &Forwarder{}
	err := f.parseOption("priority=10&weight=50&cascade=2&interface=eth0")
	if err != nil {
		t.Fatalf("parseOption error: %s", err)
	}
	if p := f.Priority(); p != 10 {
		t.Errorf("priority: got %d, want 10", p)
	}
	if w := f.Weight(); w != 50 {
		t.Errorf("weight: got %d, want 50", w)
	}
	if c := f.Cascade(); c != 2 {
		t.Errorf("cascade: got %d, want 2", c)
	}
	if f.intface != "eth0" {
		t.Errorf("interface: got %q, want eth0", f.intface)
	}
}

func TestParseOptionPriorityParseError(t *testing.T) {
	f := &Forwarder{}
	err := f.parseOption("priority=abc&weight=10")
	if err == nil {
		t.Fatal("expected parse error for invalid priority, got nil")
	}
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError, got %T: %s", err, err)
	}
	// Verify weight was NOT set (parse exits early on priority error)
	if w := f.Weight(); w != 1 {
		t.Errorf("expected default weight 1 after parse failure, got %d", w)
	}
}

func TestParseOptionWeightParseError(t *testing.T) {
	f := &Forwarder{}
	f.SetPriority(5)
	err := f.parseOption("weight=abc")
	if err == nil {
		t.Fatal("expected parse error for invalid weight, got nil")
	}
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError, got %T: %s", err, err)
	}
	if p := f.Priority(); p != 5 {
		t.Errorf("expected priority 5 preserved, got %d", p)
	}
}

func TestParseOptionCascadeParseError(t *testing.T) {
	f := &Forwarder{}
	err := f.parseOption("cascade=abc")
	if err == nil {
		t.Fatal("expected parse error for invalid cascade, got nil")
	}
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError, got %T: %s", err, err)
	}
}

func TestForwarderWeightDefault(t *testing.T) {
	f := &Forwarder{}
	if w := f.Weight(); w != 1 {
		t.Errorf("default weight: got %d, want 1", w)
	}
}

func TestForwarderWeightExplicitZeroStaysZero(t *testing.T) {
	f := newTestForwarder(t, "testok://weight-zero#weight=0")
	if w := f.Weight(); w != 0 {
		t.Errorf("explicit zero weight: got %d, want 0", w)
	}
}

func TestForwarderOptionsTrackWeightAndCascade(t *testing.T) {
	f := newTestForwarder(t, "testok://track#weight=0&cascade=2")
	if f.Weight() != 0 {
		t.Fatalf("weight = %d, want explicit 0", f.Weight())
	}
	if f.Cascade() != 2 {
		t.Fatalf("cascade = %d, want 2", f.Cascade())
	}

	f = newTestForwarder(t, "testok://track-default")
	if f.Weight() != 1 {
		t.Fatalf("default weight = %d, want 1", f.Weight())
	}
	if f.Cascade() != 0 {
		t.Fatalf("cascade = %d, want 0 when the option is absent", f.Cascade())
	}
}

func TestForwarderChainAddr(t *testing.T) {
	f := newTestForwarder(t, "testok://chain-a,testok://chain-b")
	if got := f.Addr(); got != "chain-a,chain-b" {
		t.Fatalf("addr = %q, want %q", got, "chain-a,chain-b")
	}
}
