package rule

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	_ "github.com/nadoo/glider/proxy/smux"
	_ "github.com/nadoo/glider/proxy/unix"
)

func TestNewProxyCascadeDetectionAllowsOptionOrder(t *testing.T) {
	p := NewProxy([]string{"testok://order#weight=20&cascade=1"}, &Strategy{Strategy: "pr"}, nil)
	if p.cascade == nil {
		t.Fatal("expected cascade mode when cascade option follows weight")
	}
}

func TestParseMainForwardersRejectsMissingCascadeInCascadeMode(t *testing.T) {
	_, _, err := parseMainForwarders([]string{
		"testok://mc-1#cascade=1",
		"testok://mc-2#weight=20",
	}, &Strategy{Strategy: "pr"})
	if err == nil {
		t.Fatal("expected missing cascade option to fail when cascade is used")
	}
	if !strings.Contains(err.Error(), "cascade > 0") {
		t.Fatalf("error = %v, want cascade > 0 message", err)
	}
}

func TestParseMainForwardersRejectsZeroCascadeInCascadeMode(t *testing.T) {
	_, _, err := parseMainForwarders([]string{
		"testok://zc-1#cascade=1",
		"testok://zc-2#cascade=0&weight=20",
	}, &Strategy{Strategy: "pr"})
	if err == nil {
		t.Fatal("expected cascade=0 to fail when cascade is used")
	}
	if !strings.Contains(err.Error(), "cascade > 0") {
		t.Fatalf("error = %v, want cascade > 0 message", err)
	}
}

func TestProxyNextDialerNeverReturnsNil(t *testing.T) {
	p := NewProxy([]string{"testok://proxy-next#cascade=1"}, &Strategy{Strategy: "pr"}, nil)
	d := p.NextDialer("target:80")
	if d == nil {
		t.Fatal("Proxy.NextDialer returned nil")
	}
	if d.Addr() == "" {
		t.Fatal("Proxy.NextDialer returned dialer with empty Addr")
	}
}

func TestProxyNextDialerNoTypedNilWithoutCascade(t *testing.T) {
	p := NewProxy([]string{"testok://typed-nil"}, &Strategy{Strategy: "prs"}, nil)
	d := p.NextDialer("target:80")
	if _, ok := d.(*errorDialer); !ok {
		t.Fatalf("dialer = %T, want *errorDialer", d)
	}
	if _, err := d.Dial("tcp", "target:80"); !errors.Is(err, errNoAvailableForwarder) {
		t.Fatalf("Dial error = %v, want %v", err, errNoAvailableForwarder)
	}
}

func TestNewProxyDoesNotRewriteMainStrategy(t *testing.T) {
	s := &Strategy{Strategy: "pr"}
	NewProxy([]string{"testok://mutate#cascade=1"}, s, nil)
	if s.Strategy != "pr" {
		t.Fatalf("main strategy mutated to %q", s.Strategy)
	}
}

func TestProxyRecordForwarderFailure(t *testing.T) {
	p := &Proxy{}
	f := newTestForwarder(t, "testok://record-a#cascade=1")
	f.Enable()

	p.Record(f, false)
	if f.Failures() != 1 {
		t.Fatalf("failures = %d, want 1", f.Failures())
	}

	p.Record(f, true)
	if !f.Enabled() || f.Failures() != 0 {
		t.Fatalf("enabled=%v failures=%d, want enabled with 0 failures", f.Enabled(), f.Failures())
	}
}

func TestProxyRecordCascadeDoesNotPunishHops(t *testing.T) {
	f := newTestForwarder(t, "testok://record-b#cascade=1")
	f.Enable()
	cd := &cascadeDialer{Dialer: f.Dialer, selected: []*Forwarder{f}}
	p := &Proxy{}

	p.Record(cd, false)
	if f.Failures() != 0 || !f.Enabled() {
		t.Fatalf("enabled=%v failures=%d, want enabled with 0 failures", f.Enabled(), f.Failures())
	}

	f.Disable()
	p.Record(cd, true)
	if !f.Enabled() {
		t.Fatal("successful relay must re-enable hops")
	}
}

func TestParseMainForwardersRejectsCascadeWithoutWeightedStrategy(t *testing.T) {
	_, _, err := parseMainForwarders([]string{"testok://wrong-strategy#cascade=1"}, &Strategy{Strategy: "rr"})
	if err == nil {
		t.Fatal("accepted cascade with round robin")
	}
}

// newTestRuleProxy builds a rule proxy with one rule group that matches a
// single IP, one CIDR and one domain, all served by their own test forwarder.
func newTestRuleProxy(t *testing.T) *Proxy {
	t.Helper()
	return NewProxy([]string{"testok://route-main"}, &Strategy{Strategy: "rr", Check: "disable"}, []*Config{{
		RulePath: "route.rule",
		Forward:  []string{"testok://route-rule"},
		IP:       []string{"10.9.9.9"},
		CIDR:     []string{"192.168.10.0/24"},
		Domain:   []string{"route.test"},
		Strategy: Strategy{Strategy: "rr", Check: "disable"},
	}})
}

func TestProxyFindDialerRouting(t *testing.T) {
	p := newTestRuleProxy(t)
	p.Check()

	for _, tc := range []struct {
		name, addr, want string
	}{
		{"exact ip", "10.9.9.9:80", "route-rule"},
		{"cidr range", "192.168.10.77:80", "route-rule"},
		{"domain", "route.test:443", "route-rule"},
		{"subdomain", "a.route.test:80", "route-rule"},
		{"unmatched", "other.test:80", "route-main"},
	} {
		d := p.findDialer(tc.addr)
		if got := d.Forwarders()[0].Addr(); got != tc.want {
			t.Errorf("%s: routed to %s, want %s", tc.name, got, tc.want)
		}
	}

	// An address without a port cannot be routed and falls back to main.
	if d := p.findDialer("no-port"); d.Forwarders()[0].Addr() != "route-main" {
		t.Errorf("unroutable address went to %s, want main", d.Forwarders()[0].Addr())
	}
}

func TestProxyAddDomainIP(t *testing.T) {
	p := newTestRuleProxy(t)
	p.Check()

	ip := netip.AddrFrom4([4]byte{10, 8, 8, 8})
	if err := p.AddDomainIP("route.test", ip); err != nil {
		t.Fatalf("AddDomainIP: %v", err)
	}
	if d := p.findDialer(ip.String() + ":80"); d.Forwarders()[0].Addr() != "route-rule" {
		t.Fatalf("resolved ip routed to %s, want rule group", d.Forwarders()[0].Addr())
	}
}

func TestProxyRuleOverridesCascade(t *testing.T) {
	p := NewProxy([]string{"testfail://main-chain#cascade=1"}, &Strategy{Strategy: "prs", Check: "disable"}, []*Config{{
		RulePath: "test.rule",
		Forward:  []string{"testok://rule-override"},
		Domain:   []string{"rule.test"},
		Strategy: Strategy{Strategy: "rr", Check: "disable"},
	}})
	p.Check()
	c, d, err := p.Dial("tcp", "rule.test:80")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if d.Addr() != "rule-override" {
		t.Fatalf("dialer=%s", d.Addr())
	}
}

func TestCascadeTransportPlacement(t *testing.T) {
	for _, tc := range []struct {
		forwards []string
		invalid  bool
	}{
		{[]string{"smux://127.0.0.1:1#cascade=1", "testok://later#cascade=2"}, false},
		{[]string{"testok://first#cascade=1", "smux://127.0.0.1:1#cascade=2"}, true},
		{[]string{"unix:///unused.sock,testok://first#cascade=1"}, false},
		{[]string{"testok://first#cascade=1", "unix:///unused.sock#cascade=2"}, true},
		{[]string{"testok://first,unix:///unused.sock#cascade=1"}, true},
	} {
		_, _, err := parseMainForwarders(tc.forwards, &Strategy{Strategy: "prs"})
		if (err != nil) != tc.invalid {
			t.Fatalf("forwards=%v error=%v", tc.forwards, err)
		}
	}
}
