package rule

import "testing"

func TestParseCascade(t *testing.T) {
	for _, raw := range []string{
		"cascade=1&attempts=6&retrydelay=100", "cascade=4294967295&attempts=100&retries=100&retrydelay=5000&attemptdelay=5000", "cascade=2&retries=0&retrydelay=0", "attempts=6", "attemptdelay=0",
	} {
		s := &Strategy{}
		if err := s.ParseCascade([]string{raw}); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(s.Cascade) != 1 {
			t.Fatalf("cascades=%v", s.Cascade)
		}
	}
	for _, raw := range []string{
		"", "cascade=1", "cascade=&attempts=3", "cascade=0&attempts=1", "cascade=-1&attempts=1", "cascade=4294967296&attempts=1", "cascade=x&attempts=1", "cascade=1&cascade=2&attempts=1",
		"attempts=0", "attempts=101", "attempts=", "attempts=1&attempts=2", "retries=101", "retries=-1", "retrydelay=5001", "retrydelay=-1", "retrydelay=", "attemptdelay=5001", "attemptdelay=-1", "attemptdelay=", "attemptdelay=x", "attemptdelay=0&attemptdelay=1", "attempts=%", "1:attempts=3", "level=1&attempts=3", "cacade=1&attempts=3",
	} {
		if err := (&Strategy{}).ParseCascade([]string{raw}); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestCascadeOptionsMerge(t *testing.T) {
	for _, prefix := range []string{"", "cascade=1&"} {
		s := &Strategy{}
		if err := s.ParseCascade([]string{prefix + "retries=6&retrydelay=100", prefix + "attempts=6&attemptdelay=0"}); err != nil {
			t.Fatal(err)
		}
		for _, o := range s.Cascade {
			if o.Attempts == nil || *o.Attempts != 6 || o.Retries == nil || *o.Retries != 6 || o.RetryDelay == nil || *o.RetryDelay != 100 || o.AttemptDelay == nil || *o.AttemptDelay != 0 {
				t.Fatalf("options=%+v", o)
			}
		}
		for _, key := range []string{"attempts", "retries", "retrydelay", "attemptdelay"} {
			if err := (&Strategy{}).ParseCascade([]string{prefix + key + "=1", prefix + key + "=2"}); err == nil {
				t.Fatalf("accepted duplicate %s", key)
			}
		}
		for _, key := range []string{"weight", "priority", "interface", "level", "unknown"} {
			if err := (&Strategy{}).ParseCascade([]string{prefix + key + "=1"}); err == nil {
				t.Fatalf("accepted %s", key)
			}
		}
	}
	s := &Strategy{}
	if err := s.ParseCascade([]string{"cascade=1&retrydelay=0"}); err != nil {
		t.Fatal(err)
	}
	o := s.Cascade[1]
	if o.Attempts != nil || o.Retries != nil || o.AttemptDelay != nil || o.RetryDelay == nil || *o.RetryDelay != 0 {
		t.Fatalf("options=%+v", o)
	}
}

func TestCascadeOverrideRequiresMainLevel(t *testing.T) {
	s := &Strategy{Strategy: "prs"}
	if err := s.ParseCascade([]string{"cascade=2&attempts=6"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseMainForwarders([]string{"testok://unknown-level#cascade=1"}, s); err == nil {
		t.Fatal("accepted override for missing cascade")
	}
	if _, _, err := parseMainForwarders(nil, s); err == nil {
		t.Fatal("accepted override without forwarders")
	}
}

func TestGlobalCascadeOptions(t *testing.T) {
	for _, raw := range []string{"attempts=6", "retries=0", "retrydelay=0", "attemptdelay=0", "attempts=6&retries=2&retrydelay=100"} {
		s := &Strategy{Strategy: "prs"}
		if err := s.ParseCascade([]string{raw}); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if _, _, err := parseMainForwarders([]string{"testok://global-options#cascade=1"}, s); err != nil {
			t.Fatal(err)
		}
		if _, _, err := parseMainForwarders(nil, s); err == nil {
			t.Fatal("accepted global override without a cascade")
		}
	}
}
