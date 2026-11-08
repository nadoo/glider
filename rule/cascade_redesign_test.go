package rule

import (
	"errors"
	"net"
	"testing"
	"time"

	_ "github.com/nadoo/glider/proxy/trojan"
	_ "github.com/nadoo/glider/proxy/udp"
)

func TestCascadeReturnsSuccessfulConnectionWithoutRedial(t *testing.T) {
	for _, udp := range []bool{false, true} {
		f := newTestForwarder(t, "testok://single-dial#cascade=1")
		cg := newTestCascade(t, "prs", f)
		resetTestDialCount(t, "single-dial")
		if udp {
			c, _, err := cg.DialUDP("udp", "target:53")
			if err != nil {
				t.Fatal(err)
			}
			c.Close()
		} else {
			c, _, err := cg.Dial("tcp", "target:80")
			if err != nil {
				t.Fatal(err)
			}
			c.Close()
		}
		if n := testDialCount(t, "single-dial"); n != 1 {
			t.Errorf("udp=%v dials=%d, want 1", udp, n)
		}
	}
}

func TestCascadeNextDialerRequiresEveryStrictLevel(t *testing.T) {
	down := newTestForwarder(t, "testok://strict-missing#cascade=1")
	up := newTestForwarder(t, "testok://strict-present#cascade=2")
	up.Enable()
	cg := NewCascadeGroup([]*Forwarder{down, up}, &Strategy{Strategy: "prs", DialTimeout: 1})
	c, err := cg.NextDialer("target:80").Dial("tcp", "target:80")
	if c != nil {
		c.Close()
	}
	if !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("error=%v, want no available forwarder", err)
	}
}

func TestPRZeroWeightDoesNotHideLowerPriority(t *testing.T) {
	zero := newTestForwarder(t, "testok://priority-zero#weight=0&priority=100")
	positive := newTestForwarder(t, "testok://priority-positive#weight=1&priority=1")
	g := newTestGroup(t, "pr", zero, positive)
	if got := g.NextDialer(""); got != positive {
		t.Fatalf("selected=%v, want positive weight", got)
	}
}

func TestPRSingleZeroWeightIsNotSelected(t *testing.T) {
	zero := newTestForwarder(t, "testok://single-zero#weight=0")
	g := newTestGroup(t, "pr", zero)
	if got := g.NextDialer(""); got != nil {
		t.Fatalf("selected=%v, want nil", got)
	}
}

func TestCascadeRetryBudgetAndAdapter(t *testing.T) {
	for _, udp := range []bool{false, true} {
		f := newTestForwarder(t, "testfail://budget#cascade=1")
		cg := newTestCascadeWithAttempts(t, "prs", 3, f)
		f.retries = 2
		f.retryDelay = 0
		resetTestDialCount(t, "budget")
		d := cg.NextDialer("target:80")
		var err error
		if udp {
			_, err = d.DialUDP("udp", "target:53")
		} else {
			_, err = d.Dial("tcp", "target:80")
		}
		if !errors.Is(err, errTestDial) {
			t.Fatalf("error=%v", err)
		}
		if n := testDialCount(t, "budget"); n != 9 {
			t.Fatalf("udp=%v calls=%d, want 9", udp, n)
		}
	}
}

func TestCascadeDisabledZeroWeightLevelIsAbsent(t *testing.T) {
	zero := newTestForwarder(t, "testok://absent-disabled#cascade=1&weight=0")
	up := newTestForwarder(t, "testok://present-enabled#cascade=2")
	up.Enable()
	cg := NewCascadeGroup([]*Forwarder{zero, up}, &Strategy{Strategy: "prs"})
	c, d, err := cg.Dial("tcp", "target:80")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if d.Addr() != "cascade: present-enabled" {
		t.Fatalf("dialer=%s", d.Addr())
	}
}

func TestCascadeAdapterKeepsSelectionPerCall(t *testing.T) {
	f := newTestForwarder(t, "testok://adapter-health#cascade=1")
	cg := newTestCascade(t, "prs", f)
	d := cg.NextDialer("target:80")
	c, err := d.Dial("tcp", "target:80")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	f.Disable()
	if _, err := d.Dial("tcp", "target:80"); !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("adapter reused unavailable chain: %v", err)
	}
}

func TestCascadeUDPStreamDoesNotFallBackToRawUDP(t *testing.T) {
	f := newTestForwarder(t, "testfail://udp-stream-fallback#cascade=1")
	cg := newTestCascadeWithAttempts(t, "pr", 1, f)
	c, _, err := cg.Dial("udp", "127.0.0.1:9")
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatal("returned raw UDP after a framed stream failed")
	}
}

func TestCascadeUDPFailureThroughProtocolWrappers(t *testing.T) {
	for _, raw := range []string{
		"testfail://trojan-wrapper,trojanc://test@127.0.0.1:1#cascade=1",
		"testfail://udp-wrapper,udp://127.0.0.1:1#cascade=1",
	} {
		f := newTestForwarder(t, raw)
		cg := newTestCascadeWithAttempts(t, "prs", 1, f)
		pc, _, err := cg.DialUDP("udp", "127.0.0.1:9")
		if pc != nil || err == nil {
			t.Fatalf("packet connection=%v error=%v", pc, err)
		}
	}
}

func TestCascadePacketFallbackDeliversDatagram(t *testing.T) {
	target, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	target.SetReadDeadline(time.Now().Add(time.Second))
	f := newTestForwarder(t, "testfail://packet-fallback#cascade=1")
	cg := newTestCascadeWithAttempts(t, "pr", 1, f)
	pc, _, err := cg.DialUDP("udp", target.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if _, err := pc.WriteTo([]byte("payload"), target.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _, err := target.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "payload" {
		t.Fatalf("payload=%q error=%v", buf[:n], err)
	}
}
