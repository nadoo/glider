package rule

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCascadeSingleLevel(t *testing.T) {
	f := newTestForwarder(t, "testok://one#cascade=1&weight=50")
	cg := newTestCascade(t, "pr", f)

	if testCascadeChain(t, cg) == nil {
		t.Fatal("nextDialer returned nil for single cascade level")
	}
}

func TestCascadeTwoLevels(t *testing.T) {
	f1 := newTestForwarder(t, "testok://two-1#cascade=1&weight=100")
	f2 := newTestForwarder(t, "testok://two-2#cascade=2&weight=100")
	cg := newTestCascade(t, "pr", f1, f2)

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer for two levels")
	}
	if len(cd.selected) != 2 {
		t.Fatalf("selected forwarders: got %d, want 2", len(cd.selected))
	}
	if got := testDialBase(t, "two-2"); got != "two-1" {
		t.Fatalf("second hop base = %q, want %q", got, "two-1")
	}
}

func TestCascadeSkipsEmptyLevel(t *testing.T) {
	f1 := newTestForwarder(t, "testok://skip-1#cascade=1&weight=100")
	f2 := newTestForwarder(t, "testok://skip-2#cascade=2&weight=0")
	f3 := newTestForwarder(t, "testok://skip-3#cascade=2&weight=0")
	cg := newTestCascade(t, "pr", f1, f2, f3)

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer")
	}
	if cd.Dialer == nil {
		t.Fatal("cascade dialer with nil embedded dialer")
	}
	if len(cd.selected) != 1 || cd.selected[0] != f1 {
		t.Fatalf("selected forwarders = %v, want only cascade level 1", cd.selected)
	}
}

func TestCascadeAllLevelsSkipped(t *testing.T) {
	f1 := newTestForwarder(t, "testok://allskip-1#cascade=1&weight=0")
	f2 := newTestForwarder(t, "testok://allskip-2#cascade=2&weight=0")
	cg := newTestCascade(t, "pr", f1, f2)

	// pr: all levels skipped -> returns direct dialer (not nil)
	if nd := testCascadeChain(t, cg); nd == nil {
		t.Fatal("expected non-nil dialer (direct) when all levels are skipped in pr mode")
	}
}

func TestCascadeAllLevelsSkippedPrsReturnsError(t *testing.T) {
	f1 := newTestForwarder(t, "testok://allskip-prs-1#cascade=1&weight=0")
	f2 := newTestForwarder(t, "testok://allskip-prs-2#cascade=2&weight=0")
	cg := newTestCascade(t, "prs", f1, f2)

	nd := testCascadeChain(t, cg)
	if _, ok := nd.(*errorDialer); !ok {
		t.Fatalf("got %T, want error dialer when all levels are skipped in prs mode", nd)
	}
}

func TestCascadeSelectsOneForwarderPerPositiveLevel(t *testing.T) {
	level1 := newTestForwarder(t, "testok://sel-1#cascade=1&weight=100")
	level2 := newTestForwarder(t, "testok://sel-2#cascade=2&weight=100")
	cg := newTestCascade(t, "pr", level1, level2)

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer")
	}
	if len(cd.selected) != 2 {
		t.Fatalf("selected %d forwarders, want 2", len(cd.selected))
	}
}

func TestCascadeSkipsAllZeroWeightLevel(t *testing.T) {
	level1 := newTestForwarder(t, "testok://zero-lvl-1#cascade=1&weight=100")
	level2 := newTestForwarder(t, "testok://zero-lvl-2#cascade=2&weight=0")
	cg := newTestCascade(t, "pr", level1, level2)

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer")
	}
	if len(cd.selected) != 1 || cd.selected[0] != level1 {
		t.Fatalf("selected = %v, want only cascade level 1", cd.selected)
	}
}

func TestCascadeLevelOrder(t *testing.T) {
	f10 := newTestForwarder(t, "testok://lvl-10#cascade=10&weight=100")
	f2 := newTestForwarder(t, "testok://lvl-2#cascade=2&weight=100")
	cg := newTestCascade(t, "pr", f10, f2)

	if len(cg.levels) != 2 {
		t.Fatalf("levels = %d, want 2", len(cg.levels))
	}
	if cg.levels[0].name != "cascade-2" || cg.levels[1].name != "cascade-10" {
		t.Fatalf("level order = %s, %s; want cascade-2, cascade-10",
			cg.levels[0].name, cg.levels[1].name)
	}
}

func TestCascadeFirstHopUsesOwnDialer(t *testing.T) {
	f := newTestForwarder(t, "testok://hop-a#cascade=1&weight=100")
	cg := newTestCascade(t, "pr", f)

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer")
	}
	if cd.Dialer != f.Dialer {
		t.Fatal("first hop must use the forwarder's own dialer so #interface= applies")
	}
}

func TestCascadePathDistribution(t *testing.T) {
	l1a := newTestForwarder(t, "testok://path-1a#cascade=1&weight=20")
	l1b := newTestForwarder(t, "testok://path-1b#cascade=1&weight=80")
	l2c := newTestForwarder(t, "testok://path-2c#cascade=2&weight=30")
	l2d := newTestForwarder(t, "testok://path-2d#cascade=2&weight=70")
	cg := newTestCascade(t, "pr", l1a, l1b, l2c, l2d)

	type path struct {
		first, second *Forwarder
		probability   float64
	}
	paths := []path{
		{l1a, l2c, 0.2 * 0.3},
		{l1a, l2d, 0.2 * 0.7},
		{l1b, l2c, 0.8 * 0.3},
		{l1b, l2d, 0.8 * 0.7},
	}

	const samples = 50000
	counts := make(map[[2]*Forwarder]int64)
	for i := 0; i < samples; i++ {
		cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
		if !ok || len(cd.selected) != 2 {
			t.Fatal("unexpected cascade dialer for two levels")
		}
		counts[[2]*Forwarder{cd.selected[0], cd.selected[1]}]++
	}

	chi2 := 0.0
	for _, p := range paths {
		key := [2]*Forwarder{p.first, p.second}
		expected := float64(samples) * p.probability
		diff := float64(counts[key]) - expected
		chi2 += diff * diff / expected
		t.Logf("%s -> %s: observed=%d expected=%.0f", p.first.Addr(), p.second.Addr(), counts[key], expected)
	}

	// Three degrees of freedom. For independent per-level draws, chi2 > 50 has
	// a probability below 1e-9; a random false positive remains possible.
	t.Logf("chi2=%.2f (df=3, threshold 50)", chi2)
	if chi2 > 50 {
		t.Fatalf("cascade path distribution off: chi2=%.2f, want <= 50", chi2)
	}
}

func TestCascadeDialRetriesSameForwarder(t *testing.T) {
	f := newTestForwarder(t, "testflaky://retry#cascade=1&weight=100")
	cg := newTestCascadeWithAttempts(t, "prs", 1, f)
	f.retries = 1
	f.retryDelay = 0
	resetTestDialCount(t, "retry")

	c, _, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()

	if n := testDialCount(t, "retry"); n != 2 {
		t.Fatalf("dials = %d, want 2 (failed dial and successful retry)", n)
	}
}

func TestCascadeDialNoRetriesFails(t *testing.T) {
	f := newTestForwarder(t, "testflaky://noretry#cascade=1&weight=100")
	cg := newTestCascadeWithAttempts(t, "prs", 1, f)
	resetTestDialCount(t, "noretry")

	if _, _, err := cg.Dial("tcp", "target:80"); err == nil {
		t.Fatal("expected error without retries and a single attempt")
	}
	if n := testDialCount(t, "noretry"); n != 1 {
		t.Fatalf("dials = %d, want 1", n)
	}
}

func TestCascadeDialAttemptsRedraw(t *testing.T) {
	f := newTestForwarder(t, "testflaky://redraw#cascade=1&weight=100")
	cg := newTestCascadeWithAttempts(t, "prs", 2, f)
	resetTestDialCount(t, "redraw")

	c, _, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()

	if n := testDialCount(t, "redraw"); n != 2 {
		t.Fatalf("dials = %d, want 2 (failed draw and successful draw)", n)
	}
}

func TestCascadeDialZeroWeightLevelSkipped(t *testing.T) {
	skip := newTestForwarder(t, "testfail://zero-skip#cascade=1&weight=0")
	use := newTestForwarder(t, "testok://zero-use#cascade=2&weight=100")
	cg := newTestCascade(t, "prs", skip, use)
	resetTestDialCount(t, "zero-skip")
	resetTestDialCount(t, "zero-use")

	c, _, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()

	if n := testDialCount(t, "zero-skip"); n != 0 {
		t.Fatalf("skipped level dialed %d times, want 0", n)
	}
	if n := testDialCount(t, "zero-use"); n != 1 {
		t.Fatalf("used level dialed %d times, want 1", n)
	}
}

func TestCascadeDialAllLevelsSkippedPrsErrors(t *testing.T) {
	f1 := newTestForwarder(t, "testok://dial-allskip-prs-1#cascade=1&weight=0")
	f2 := newTestForwarder(t, "testok://dial-allskip-prs-2#cascade=2&weight=0")
	cg := newTestCascade(t, "prs", f1, f2)

	_, _, err := cg.Dial("tcp", "target:80")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
}

func TestCascadeDialDefaultAttempts(t *testing.T) {
	f := newTestForwarder(t, "testfail://default-attempts#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)
	resetTestDialCount(t, "default-attempts")

	if _, _, err := cg.Dial("tcp", "target:80"); err == nil {
		t.Fatal("expected error when forwarder rejects")
	}
	if n := testDialCount(t, "default-attempts"); n != 1 {
		t.Fatalf("dials = %d, want 1 default attempt", n)
	}
}

func TestCascadeDialAttemptsTryAnotherForwarder(t *testing.T) {
	bad := newTestForwarder(t, "testfail://multi-bad#cascade=1&weight=1")
	good := newTestForwarder(t, "testok://multi-good#cascade=1&weight=1")
	cg := newTestCascadeWithAttempts(t, "prs", 50, bad, good)

	// Each draw picks one of the two forwarders, so the healthy one is picked
	// with probability 1 - (1/2)^50.
	c, _, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()
}

func TestCascadeDialDisabledLevelAbortsPRS(t *testing.T) {
	down := newTestForwarder(t, "testok://disabled-l1#cascade=1&weight=100")
	up := newTestForwarder(t, "testok://disabled-l2#cascade=2&weight=100")
	up.Enable()
	cg := NewCascadeGroup([]*Forwarder{down, up}, &Strategy{Strategy: "prs", DialTimeout: 1})

	_, _, err := cg.Dial("tcp", "target:80")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
}

func TestCascadeDialDisabledLevelSkippedPR(t *testing.T) {
	down := newTestForwarder(t, "testok://pr-disabled-l1#cascade=1&weight=100")
	up := newTestForwarder(t, "testok://pr-disabled-l2#cascade=2&weight=100")
	up.Enable()
	cg := NewCascadeGroup([]*Forwarder{down, up}, &Strategy{Strategy: "pr", DialTimeout: 1})
	resetTestDialCount(t, "pr-disabled-l2")

	c, dialer, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()

	if n := testDialCount(t, "pr-disabled-l2"); n != 1 {
		t.Fatalf("used level dialed %d times, want 1", n)
	}
	cd, ok := dialer.(*cascadeDialer)
	if !ok || len(cd.selected) != 1 || cd.selected[0] != up {
		t.Fatalf("selected = %v, want only the enabled level", cd)
	}
}

func TestCascadeDialPRFallsBackToDirectWhenChainFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	healthy := newTestForwarder(t, "testok://keep-ok#cascade=1&weight=100")
	broken := newTestForwarder(t, "testfail://keep-bad#cascade=2&weight=100")
	cg := newTestCascade(t, "pr", healthy, broken)
	resetTestDialCount(t, "keep-ok")
	resetTestDialCount(t, "keep-bad")
	c, dialer, err := cg.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if dialer.Addr() != "DIRECT" {
		t.Fatalf("dialer=%s, want DIRECT", dialer.Addr())
	}
	if n := testDialCount(t, "keep-ok"); n != 0 {
		t.Fatalf("partial chain dialed %d times", n)
	}
	if n := testDialCount(t, "keep-bad"); n != 1 {
		t.Fatalf("chain dials=%d, want 1", n)
	}
}

func TestCascadeDialDirectFallbackPR(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	down := newTestForwarder(t, "testok://fallback-l1#cascade=1&weight=100")
	cg := NewCascadeGroup([]*Forwarder{down}, &Strategy{Strategy: "pr", DialTimeout: 1})

	c, dialer, err := cg.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("pr must fall back to direct: %s", err)
	}
	c.Close()

	if n := testDialCount(t, "fallback-l1"); n != 0 {
		t.Fatalf("disabled level dialed %d times, want 0", n)
	}
	cd, ok := dialer.(*cascadeDialer)
	if !ok || len(cd.selected) != 0 {
		t.Fatalf("dialer = %#v, want cascade dialer without hops", dialer)
	}
}

func TestCascadeDialDirectFallbackPRSErrors(t *testing.T) {
	down := newTestForwarder(t, "testok://strict-down#cascade=1&weight=100")
	cg := NewCascadeGroup([]*Forwarder{down}, &Strategy{Strategy: "prs", DialTimeout: 1})

	_, _, err := cg.Dial("tcp", "127.0.0.1:1")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
}

func TestCascadeDialNewDirectFailure(t *testing.T) {
	f := newTestForwarder(t, "testok://bad-iface#cascade=1&weight=100")
	cg := NewCascadeGroup([]*Forwarder{f}, &Strategy{
		Strategy:    "pr",
		IntFace:     "glider-test-missing-iface",
		DialTimeout: 1,
	})

	_, dialer, err := cg.Dial("tcp", "target:80")
	if err == nil {
		t.Fatal("expected error for unknown interface")
	}
	if !strings.Contains(err.Error(), "glider-test-missing-iface") {
		t.Fatalf("error = %v, want unknown interface error", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil error dialer")
	}
	if n := testDialCount(t, "bad-iface"); n != 0 {
		t.Fatalf("dialed %d times despite bad interface, want 0", n)
	}
}

func TestCascadeDialNoAvailableReturnsError(t *testing.T) {
	f := newTestForwarder(t, "testok://cascade-no-avail#cascade=1")
	cg := NewCascadeGroup([]*Forwarder{f}, &Strategy{Strategy: "prs", DialTimeout: 1})

	conn, dialer, err := cg.Dial("tcp", "127.0.0.1:1")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
	if conn != nil || dialer == nil {
		t.Fatalf("conn=%v dialer=%v, want nil conn and non-nil error dialer", conn, dialer)
	}
}

func TestCascadeDialUDPNoAvailableReturnsError(t *testing.T) {
	f := newTestForwarder(t, "testok://cascade-no-avail-udp#cascade=1")
	cg := NewCascadeGroup([]*Forwarder{f}, &Strategy{Strategy: "prs", DialTimeout: 1})

	pc, dialer, err := cg.DialUDP("udp", "127.0.0.1:1")
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error = %v, want %v", err, errNoAvailableForwarder)
	}
	if pc != nil || dialer == nil {
		t.Fatalf("packetConn=%v dialer=%v, want nil packetConn and non-nil error dialer", pc, dialer)
	}
}

func TestCascadeDialFailureDoesNotIncrementForwarderFailures(t *testing.T) {
	f := newTestForwarder(t, "testfail://nofail#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)

	if _, _, err := cg.Dial("tcp", "target:80"); err == nil {
		t.Fatal("expected error when forwarder rejects")
	}
	if f.Failures() != 0 {
		t.Fatalf("failures = %d, want 0 (retries don't punish)", f.Failures())
	}
}

func TestCascadeDialUDPFailureNoCollateral(t *testing.T) {
	f := newTestForwarder(t, "testfail://udp-nofail#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)

	if _, _, err := cg.DialUDP("udp", "target:53"); err == nil {
		t.Fatal("expected error when forwarder rejects")
	}
	if f.Failures() != 0 {
		t.Fatalf("failures = %d, want 0", f.Failures())
	}
}

func TestCascadeDialSuccessResetsFailures(t *testing.T) {
	f := newTestForwarder(t, "testok://reset#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)

	f.IncFailures()
	f.IncFailures()

	c, _, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatalf("Dial: %s", err)
	}
	c.Close()

	if f.Failures() != 0 {
		t.Fatalf("failures = %d, want 0 after successful dial", f.Failures())
	}
}

func TestCascadeDialUDPRetries(t *testing.T) {
	f := newTestForwarder(t, "testflaky://udp-retry#cascade=1&weight=100")
	cg := newTestCascadeWithAttempts(t, "prs", 1, f)
	f.retries = 1
	f.retryDelay = 0
	resetTestDialCount(t, "udp-retry")

	pc, _, err := cg.DialUDP("udp", "target:53")
	if err != nil {
		t.Fatalf("DialUDP: %s", err)
	}
	pc.Close()

	if n := testDialCount(t, "udp-retry"); n != 2 {
		t.Fatalf("dials = %d, want 2 (failed dial and successful retry)", n)
	}
}

func TestCascadeDialRetryDelay(t *testing.T) {
	f := newTestForwarder(t, "testfail://delay#cascade=1&weight=100")
	cg := newTestCascadeWithAttempts(t, "prs", 1, f)
	f.retries = 2
	f.retryDelay = 100 * time.Millisecond

	start := time.Now()
	if _, _, err := cg.Dial("tcp", "target:80"); err == nil {
		t.Fatal("expected error when forwarder rejects")
	}
	elapsed := time.Since(start)

	// Two retries mean two pauses of 100ms. The fake dials fail instantly.
	if elapsed < 150*time.Millisecond {
		t.Fatalf("elapsed %v, want at least 150ms with retrydelay=100", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed %v, want below 2s", elapsed)
	}
}

func TestCascadeDialPRSStrictReturnsNonNilDialer(t *testing.T) {
	f := newTestForwarder(t, "testfail://strict#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)

	conn, dialer, err := cg.Dial("tcp", "target:80")
	if err == nil {
		t.Fatal("expected reject dial to fail")
	}
	if !errors.Is(err, errTestDial) {
		t.Fatalf("error = %v, want the dial error %v", err, errTestDial)
	}
	if conn != nil || dialer == nil {
		t.Fatalf("conn=%v dialer=%v, want nil conn and non-nil error dialer", conn, dialer)
	}
	if dialer.Addr() == "" {
		t.Fatal("expected error dialer Addr to be non-empty")
	}
	if f.Failures() != 0 {
		t.Fatalf("failures = %d, want 0 (no collateral punishment)", f.Failures())
	}
}

func TestCascadeDialUDPPRSStrictReturnsNonNilDialer(t *testing.T) {
	f := newTestForwarder(t, "testfail://strict-udp#cascade=1&weight=100")
	cg := newTestCascade(t, "prs", f)

	pc, dialer, err := cg.DialUDP("udp", "target:53")
	if err == nil {
		t.Fatal("expected reject UDP dial to fail")
	}
	if pc != nil || dialer == nil {
		t.Fatalf("packetConn=%v dialer=%v, want nil packetConn and non-nil error dialer", pc, dialer)
	}
}

func TestCascadeSingleForwarderLevelEnabledWithoutCheckDisable(t *testing.T) {
	f := newTestForwarder(t, "testok://single-cascade#cascade=1&weight=100")
	cg := NewCascadeGroup([]*Forwarder{f}, &Strategy{Strategy: "pr", DialTimeout: 1})
	cg.Check()

	if !f.Enabled() {
		t.Fatal("single-forwarder cascade level should enable the only forwarder")
	}

	cd, ok := testCascadeChain(t, cg).(*cascadeDialer)
	if !ok {
		t.Fatal("expected cascade dialer for single enabled level")
	}
	if len(cd.selected) != 1 || cd.selected[0] != f {
		t.Fatalf("selected forwarders = %v, want [%v]", cd.selected, f)
	}
}
