package rule

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
)

func TestCascadeForwarderRetries(t *testing.T) {
	for _, udp := range []bool{false, true} {
		f := newTestForwarder(t, "testfail://forwarder-retries#cascade=1&retries=2&retrydelay=0")
		cg := newTestCascade(t, "prs", f)
		for range 2 {
			resetTestDialCount(t, "forwarder-retries")
			d := cg.NextDialer("target:80")
			var err error
			if udp {
				_, err = d.DialUDP("udp", "target:53")
			} else {
				_, err = d.Dial("tcp", "target:80")
			}
			if !errors.Is(err, errTestDial) {
				t.Fatal(err)
			}
			if n := testDialCount(t, "forwarder-retries"); n != 3 {
				t.Fatalf("calls=%d, want 3", n)
			}
		}
	}
}

func TestForwarderRetryOptionValidation(t *testing.T) {
	for _, raw := range []string{"attempts=1", "attemptdelay=0", "attempts=0", "attempts=101", "attempts=-1", "attempts=", "attempts=x", "attempts=1&attempts=2", "retries=-1", "retries=101", "retries=", "retrydelay=-1", "retrydelay=5001", "retrydelay=", "retrydelay=0&retrydelay=1"} {
		if err := (&Forwarder{}).parseOption(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestCascadeLevelOverrides(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		calls int64
	}{
		{"cascade=1&attempts=6&retrydelay=0", 18},
		{"cascade=1&attempts=6&retries=0&retrydelay=0", 6},
		{"cascade=1&retrydelay=0", 3},
	} {
		f := newTestForwarder(t, "testfail://level-budget#cascade=1&retries=2&retrydelay=400")
		f.Enable()
		s := &Strategy{Strategy: "prs"}
		if err := s.ParseCascade([]string{tc.raw}); err != nil {
			t.Fatal(err)
		}
		cg := NewCascadeGroup([]*Forwarder{f}, s)
		resetTestDialCount(t, "level-budget")
		if _, _, err := cg.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
			t.Fatal(err)
		}
		if n := testDialCount(t, "level-budget"); n != tc.calls {
			t.Fatalf("%s: calls=%d want %d", tc.raw, n, tc.calls)
		}
	}
	a := newTestForwarder(t, "testfail://level-total-a#cascade=1")
	b := newTestForwarder(t, "testfail://level-total-b#cascade=1")
	a.Enable()
	b.Enable()
	s := &Strategy{Strategy: "prs"}
	if err := s.ParseCascade([]string{"cascade=1&attempts=6"}); err != nil {
		t.Fatal(err)
	}
	resetTestDialCount(t, "level-total-a")
	resetTestDialCount(t, "level-total-b")
	cg := NewCascadeGroup([]*Forwarder{a, b}, s)
	if _, _, err := cg.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
		t.Fatal(err)
	}
	if n := testDialCount(t, "level-total-a") + testDialCount(t, "level-total-b"); n != 6 {
		t.Fatalf("total calls=%d", n)
	}
}

func TestCascadeLevelsAdvanceIndependently(t *testing.T) {
	a := newTestForwarder(t, "testfail://independent-a#cascade=1&retries=1&retrydelay=10")
	b := newTestForwarder(t, "testfail://independent-b#cascade=2&retries=2&retrydelay=20")
	cg := newTestCascadeWithAttempts(t, "prs", 2, a, b)
	states := make([]cascadeSelection, 2)
	var previous *cascadeDialer
	for i, wantDelay := range []time.Duration{0, 20 * time.Millisecond, 20 * time.Millisecond, 10 * time.Millisecond} {
		chain, delay, err := cg.chain(states, previous)
		if err != nil {
			t.Fatal(err)
		}
		if delay != wantDelay {
			t.Fatalf("draw=%d delay=%v want %v", i, delay, wantDelay)
		}
		if previous != nil && chain != previous {
			t.Fatal("rebuilt unchanged chain")
		}
		previous = chain
	}
	if states[0].draws != 2 || states[1].draws != 2 || states[0].retries != 0 || states[1].retries != 2 {
		t.Fatalf("states=%+v", states)
	}
	if _, _, err := cg.chain(states, previous); !errors.Is(err, errCascadeBudget) {
		t.Fatalf("error=%v", err)
	}
}

func TestCascadeBudgetExhaustionDoesNotDropLevel(t *testing.T) {
	for _, strategy := range []string{"pr", "prs"} {
		a := newTestForwarder(t, "testfail://exhausted-a#cascade=1")
		b := newTestForwarder(t, "testfail://exhausted-b#cascade=2&retries=3&retrydelay=0")
		cg := newTestCascade(t, strategy, a, b)
		var paths []string
		_, err := cg.connect(func(d proxy.Dialer) error { paths = append(paths, d.Addr()); return errTestDial })
		if !errors.Is(err, errTestDial) {
			t.Fatal(err)
		}
		wantCalls := 1
		if strategy == "pr" {
			wantCalls = 2
		}
		if len(paths) != wantCalls || paths[0] != "cascade: exhausted-a -> exhausted-b" {
			t.Fatalf("paths=%v", paths)
		}
		if strategy == "pr" && paths[1] != "DIRECT" {
			t.Fatalf("paths=%v", paths)
		}
	}
}

func TestCascadeConcurrentBudgets(t *testing.T) {
	f := newTestForwarder(t, "testfail://concurrent-budgets#cascade=1&retries=1&retrydelay=0")
	cg := newTestCascadeWithAttempts(t, "prs", 3, f)
	d := cg.NextDialer("target:80")
	resetTestDialCount(t, "concurrent-budgets")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := testDialCount(t, "concurrent-budgets"); n != 120 {
		t.Fatalf("calls=%d", n)
	}
}

func TestCascadeGlobalOverridePrecedence(t *testing.T) {
	for _, raw := range [][]string{
		{"attempts=6&retries=2&retrydelay=100&attemptdelay=50", "cascade=1&retries=0&retrydelay=0&attemptdelay=0", "cascade=2&attempts=3"},
		{"cascade=2&attempts=3", "cascade=1&retries=0&retrydelay=0&attemptdelay=0", "attempts=6&retries=2&retrydelay=100&attemptdelay=50"},
	} {
		a := newTestForwarder(t, "testfail://global-a#cascade=1&retries=4&retrydelay=400")
		b := newTestForwarder(t, "testfail://global-b#cascade=2&retries=4&retrydelay=400")
		a.Enable()
		b.Enable()
		s := &Strategy{Strategy: "prs"}
		if err := s.ParseCascade(raw); err != nil {
			t.Fatal(err)
		}
		cg := NewCascadeGroup([]*Forwarder{a, b}, s)
		for i, want := range [][4]int{{6, 0, 0, 0}, {3, 2, 100, 50}} {
			o := cg.overrides[i]
			if o.Attempts == nil || o.Retries == nil || o.RetryDelay == nil || o.AttemptDelay == nil || *o.Attempts != want[0] || *o.Retries != want[1] || *o.RetryDelay != want[2] || *o.AttemptDelay != want[3] {
				t.Fatalf("cascade=%d overrides=%+v", i+1, o)
			}
		}
		states := make([]cascadeSelection, 2)
		var previous *cascadeDialer
		for i := range 6 {
			chain, delay, err := cg.chain(states, previous)
			if err != nil {
				t.Fatal(err)
			}
			wantDelay := 100 * time.Millisecond
			if i == 0 {
				wantDelay = 0
			} else if i%3 == 0 {
				wantDelay = 50 * time.Millisecond
			}
			if delay != wantDelay {
				t.Fatalf("try=%d delay=%v want=%v", i, delay, wantDelay)
			}
			previous = chain
		}
		if _, _, err := cg.chain(states, previous); !errors.Is(err, errCascadeBudget) {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestCascadeAttemptDelay(t *testing.T) {
	f := newTestForwarder(t, "testfail://block-delay#cascade=1&retries=2&retrydelay=10")
	f.Enable()
	s := &Strategy{Strategy: "prs"}
	if err := s.ParseCascade([]string{"cascade=1&attempts=2&attemptdelay=30"}); err != nil {
		t.Fatal(err)
	}
	cg := NewCascadeGroup([]*Forwarder{f}, s)
	states := make([]cascadeSelection, 1)
	var previous *cascadeDialer
	for i, want := range []time.Duration{0, 10 * time.Millisecond, 10 * time.Millisecond, 30 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond} {
		chain, delay, err := cg.chain(states, previous)
		if err != nil || delay != want {
			t.Fatalf("try=%d delay=%v error=%v want=%v", i, delay, err, want)
		}
		previous = chain
	}
	if _, _, err := cg.chain(states, previous); !errors.Is(err, errCascadeBudget) {
		t.Fatal(err)
	}
	resetTestDialCount(t, "block-delay")
	start := time.Now()
	if _, _, err := cg.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("elapsed=%v, want at least 70ms", elapsed)
	}
	if n := testDialCount(t, "block-delay"); n != 6 {
		t.Fatalf("calls=%d", n)
	}
}

func TestCascadeRedrawUsesCurrentWeights(t *testing.T) {
	a := newTestForwarder(t, "testok://redraw-weight-a#cascade=1&weight=1&retries=1&retrydelay=0")
	b := newTestForwarder(t, "testok://redraw-weight-b#cascade=1&weight=1&retries=2&retrydelay=0")
	cg := newTestCascadeWithAttempts(t, "prs", 3, a, b)
	b.SetWeight(0)
	states := make([]cascadeSelection, 1)
	var previous *cascadeDialer
	for i, want := range []*Forwarder{a, a, b, b, b, a, a} {
		if i == 2 {
			a.SetWeight(0)
			b.SetWeight(1)
		}
		if i == 5 {
			a.SetWeight(1)
			b.SetWeight(0)
		}
		chain, _, err := cg.chain(states, previous)
		if err != nil {
			t.Fatal(err)
		}
		if chain.selected[0] != want {
			t.Fatalf("try=%d selected=%s want=%s", i, chain.selected[0].Addr(), want.Addr())
		}
		previous = chain
	}
	if _, _, err := cg.chain(states, previous); !errors.Is(err, errCascadeBudget) {
		t.Fatal(err)
	}
}

func TestCascadeOverridesLeavePlainGroupsUnchanged(t *testing.T) {
	s := &Strategy{Strategy: "prs"}
	if err := s.ParseCascade([]string{"retries=6&retrydelay=0&attempts=6&attemptdelay=0"}); err != nil {
		t.Fatal(err)
	}
	f := newTestForwarder(t, "testfail://plain-override#retries=2&retrydelay=100")
	f.Enable()
	g := newFwdrGroup("plain", []*Forwarder{f}, s)
	resetTestDialCount(t, "plain-override")
	if _, _, err := g.Dial("tcp", "target:80"); !errors.Is(err, errTestDial) {
		t.Fatal(err)
	}
	if n := testDialCount(t, "plain-override"); n != 1 {
		t.Fatalf("plain group calls=%d", n)
	}
	if f.retries != 2 || f.retryDelay != 100*time.Millisecond {
		t.Fatal("cascade options changed plain forwarder settings")
	}
}
