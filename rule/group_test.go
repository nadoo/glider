package rule

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSchedulePREachForwarderSelected(t *testing.T) {
	fwdrs := []*Forwarder{
		newTestForwarder(t, "testok://pr-a#weight=10"),
		newTestForwarder(t, "testok://pr-b#weight=20"),
		newTestForwarder(t, "testok://pr-c#weight=30"),
	}
	g := newTestGroup(t, "pr", fwdrs...)

	seen := make(map[*Forwarder]bool)
	for i := 0; i < 200; i++ {
		seen[g.schedulePR("")] = true
	}
	if len(seen) != len(g.avail) {
		t.Errorf("expected all %d forwarders to be selected at least once, got %d", len(g.avail), len(seen))
	}
}

func TestSchedulePRSingleForwarder(t *testing.T) {
	f := newTestForwarder(t, "testok://pr-single#weight=100")
	g := newTestGroup(t, "pr", f)

	for i := 0; i < 10; i++ {
		if selected := g.schedulePR(""); selected != f {
			t.Fatal("single forwarder: must always return the only available one")
		}
	}
}

func TestSchedulePRZeroWeightNotSelected(t *testing.T) {
	f1 := newTestForwarder(t, "testok://pr-zero-1#weight=0")
	f2 := newTestForwarder(t, "testok://pr-zero-2#weight=0")
	f3 := newTestForwarder(t, "testok://pr-zero-3#weight=100")
	g := newTestGroup(t, "pr", f1, f2, f3)

	for i := 0; i < 100; i++ {
		if f := g.schedulePR(""); f == f1 || f == f2 {
			t.Fatal("schedulePR selected a zero-weight forwarder")
		}
	}
}

func TestSchedulePRSkipsDisabled(t *testing.T) {
	f1 := newTestForwarder(t, "testok://pr-disabled-1#weight=10")
	f2 := newTestForwarder(t, "testok://pr-disabled-2#weight=10")
	f3 := newTestForwarder(t, "testok://pr-disabled-3#weight=10")
	f3.Enable()
	g := newFwdrGroup("test-disabled", []*Forwarder{f1, f2, f3}, &Strategy{Strategy: "pr"})

	for i := 0; i < 100; i++ {
		if f := g.schedulePR(""); f == f1 || f == f2 {
			t.Fatal("schedulePR selected a disabled forwarder")
		}
	}
}

func TestSchedulePRAllZeroWeights(t *testing.T) {
	f1 := newTestForwarder(t, "testok://pr-allzero-1#weight=0")
	f2 := newTestForwarder(t, "testok://pr-allzero-2#weight=0")
	g := newTestGroup(t, "pr", f1, f2)

	if f := g.schedulePR(""); f != nil {
		t.Fatal("expected no forwarder when all weights are 0")
	}
}

func TestSchedulePRWeightDistribution(t *testing.T) {
	fwdrs := []*Forwarder{
		newTestForwarder(t, "testok://dist-a#weight=10"),
		newTestForwarder(t, "testok://dist-b#weight=20"),
		newTestForwarder(t, "testok://dist-c#weight=30"),
	}
	g := newTestGroup(t, "pr", fwdrs...)

	const samples = 50000
	counts := make(map[*Forwarder]int64)
	for i := 0; i < samples; i++ {
		counts[g.schedulePR("")]++
	}

	totalWeight := int64(0)
	for _, f := range fwdrs {
		totalWeight += f.Weight()
	}

	chi2 := 0.0
	for _, f := range fwdrs {
		expected := float64(samples) * float64(f.Weight()) / float64(totalWeight)
		diff := float64(counts[f]) - expected
		chi2 += diff * diff / expected
		t.Logf("%s: observed=%d expected=%.0f", f.Addr(), counts[f], expected)
	}

	// Two degrees of freedom. For a correct weighted draw, chi2 > 41.45 has a
	// probability below 1e-9; a random false positive remains possible.
	t.Logf("chi2=%.2f (df=2, threshold 41.45)", chi2)
	if chi2 > 41.45 {
		t.Fatalf("weighted draw off: chi2=%.2f, want <= 41.45", chi2)
	}
}

func TestExcludedByWeight(t *testing.T) {
	zero := newTestForwarder(t, "testok://excl-a#weight=0")
	if g := newTestGroup(t, "pr", zero); !g.excludedByWeight() {
		t.Fatal("level with only zero-weight forwarders must be excluded")
	}

	positive := newTestForwarder(t, "testok://excl-b#weight=1")
	if g := newTestGroup(t, "pr", positive); g.excludedByWeight() {
		t.Fatal("level with a positive-weight forwarder must not be excluded")
	}

	disabled := newTestForwarder(t, "testok://excl-c#weight=0")
	if g := newFwdrGroup("test", []*Forwarder{disabled}, &Strategy{Strategy: "pr"}); !g.excludedByWeight() {
		t.Fatal("disabled zero-weight level must remain excluded")
	}
}

func TestFwdrGroupDialNoAvailablePRReturnsError(t *testing.T) {
	f := newTestForwarder(t, "testok://no-avail")
	g := newFwdrGroup("test", []*Forwarder{f}, &Strategy{Strategy: "pr"})

	conn, dialer, err := g.Dial("tcp", "127.0.0.1:1")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
	if conn != nil || dialer == nil {
		t.Fatalf("conn=%v dialer=%v, want nil conn and non-nil error dialer", conn, dialer)
	}
}

func TestFwdrGroupDialUDPNoAvailablePRReturnsError(t *testing.T) {
	f := newTestForwarder(t, "testok://no-avail-udp")
	g := newFwdrGroup("test", []*Forwarder{f}, &Strategy{Strategy: "pr"})

	pc, dialer, err := g.DialUDP("udp", "127.0.0.1:1")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
	if pc != nil || dialer == nil {
		t.Fatalf("packetConn=%v dialer=%v, want nil packetConn and non-nil error dialer", pc, dialer)
	}
}

func TestCheckDisableEnablesSingleForwarderGroup(t *testing.T) {
	f := newTestForwarder(t, "testok://check-disable")
	g := newFwdrGroup("test", []*Forwarder{f}, &Strategy{Strategy: "pr", Check: "disable"})

	g.Check()

	if !f.Enabled() {
		t.Fatal("check=disable did not enable the single forwarder")
	}
	if len(g.avail) != 1 || g.avail[0] != f {
		t.Fatalf("available forwarders = %d, want the single forwarder", len(g.avail))
	}
}

func TestCheckEnablesSingleForwarder(t *testing.T) {
	f := newTestForwarder(t, "testok://single-group")
	g := newFwdrGroup("test", []*Forwarder{f}, &Strategy{Strategy: "rr"})

	g.Check()

	if !f.Enabled() {
		t.Fatal("single forwarder group must enable its only forwarder")
	}
	if len(g.avail) != 1 || g.avail[0] != f {
		t.Fatalf("available forwarders = %d, want the single forwarder", len(g.avail))
	}
}

func TestConcurrentStatusAndSelection(t *testing.T) {
	f := newTestForwarder(t, "testok://concurrent-status")
	g := newTestGroup(t, "pr", f)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				f.Disable()
				f.Enable()
				g.NextDialer("")
			}
		}()
	}
	wg.Wait()
	f.Enable()
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.avail) != 1 || g.avail[0] != f {
		t.Fatalf("available count=%d, want one forwarder", len(g.avail))
	}
}

func TestGroupDialUDPRequiresAvailableForwarder(t *testing.T) {
	// The forwarder stays disabled, so prs must refuse the UDP dial instead of
	// silently falling back to round robin.
	f := newTestForwarder(t, "testok://udp-disabled")
	g := newFwdrGroup("udp-prs", []*Forwarder{f}, &Strategy{Strategy: "prs"})

	_, d, err := g.DialUDP("udp", "target:53")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("DialUDP error = %v, want %v", err, errNoAvailableForwarder)
	}
	if _, ok := d.(*errorDialer); !ok {
		t.Fatalf("dialer = %T, want *errorDialer", d)
	}
	if _, err := d.DialUDP("udp", "target:53"); !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("errorDialer.DialUDP error = %v, want %v", err, errNoAvailableForwarder)
	}
}

func TestNewFwdrGroupDirectFallback(t *testing.T) {
	s := &Strategy{Strategy: "pr", DialTimeout: 1, RelayTimeout: 1}
	g := NewFwdrGroup("", nil, s)

	if len(g.fwdrs) != 1 {
		t.Fatalf("forwarders = %d, want the single direct fallback", len(g.fwdrs))
	}
	if g.fwdrs[0].Addr() != "DIRECT" {
		t.Fatalf("fallback addr = %q, want DIRECT", g.fwdrs[0].Addr())
	}
	// The direct fallback forces round robin on its own strategy copy.
	if g.config.Strategy != "rr" {
		t.Fatalf("fallback strategy = %q, want rr", g.config.Strategy)
	}
	if s.Strategy != "pr" {
		t.Fatalf("caller strategy mutated to %q", s.Strategy)
	}
}

// TestSetLatencyAveraging proves the moving-average formula used when
// checklatencysamples is above one.
func TestSetLatencyAveraging(t *testing.T) {
	f := newTestForwarder(t, "testok://latency-avg")
	g := newTestGroup(t, "rr", f)

	g.config.CheckLatencySamples = 4
	g.setLatency(f, 100*time.Millisecond)
	if got := f.Latency(); got != int64(100*time.Millisecond) {
		t.Fatalf("first latency = %d, want the raw sample", got)
	}

	g.setLatency(f, 200*time.Millisecond)
	want := (int64(100*time.Millisecond)*3 + int64(200*time.Millisecond)) / 4
	if got := f.Latency(); got != want {
		t.Fatalf("averaged latency = %d, want %d", got, want)
	}
}

func TestGroupPriorityAndRecovery(t *testing.T) {
	for _, strategy := range []string{"rr", "ha", "lha", "dh", "pr", "prs"} {
		high := newTestForwarder(t, "testok://priority-high#priority=10")
		low := newTestForwarder(t, "testok://priority-low#priority=1")
		g := newTestGroup(t, strategy, low, high)
		if g.NextDialer("") != high {
			t.Fatalf("%s did not select high priority", strategy)
		}
		high.Disable()
		if g.NextDialer("") != low {
			t.Fatalf("%s did not use backup", strategy)
		}
		high.Enable()
		if g.NextDialer("") != high {
			t.Fatalf("%s did not recover high priority", strategy)
		}
	}
}
