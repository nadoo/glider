package dns

import (
	"bytes"
	"net/netip"
	"testing"
	"time"
)

func TestHandleAnswerCacheTTL(t *testing.T) {
	cname := func(ttl uint32) *RR {
		var target bytes.Buffer
		if _, err := MarshalDomainTo(&target, "target.example"); err != nil {
			t.Fatal(err)
		}
		return &RR{NAME: "alias.example", TYPE: 5, CLASS: ClassINET, TTL: ttl, RDLENGTH: uint16(target.Len()), RDATA: target.Bytes()}
	}
	address := func(ttl uint32) *RR {
		ip := netip.MustParseAddr("192.0.2.1")
		return &RR{NAME: "target.example", TYPE: QTypeA, CLASS: ClassINET, TTL: ttl, RDLENGTH: 4, RDATA: ip.AsSlice()}
	}
	tests := []struct {
		name                    string
		answers                 []*RR
		minTTL, maxTTL, wantTTL int
	}{
		{"CNAME only", []*RR{cname(30)}, 0, 1800, 30},
		{"CNAME before longer address", []*RR{cname(10), address(300)}, 0, 1800, 10},
		{"CNAME after longer address", []*RR{address(300), cname(10)}, 0, 1800, 10},
		{"address expires first", []*RR{cname(300), address(10)}, 0, 1800, 10},
		{"zero TTL CNAME", []*RR{cname(0), address(300)}, 0, 1800, 0},
		{"zero TTL address", []*RR{cname(300), address(0)}, 0, 1800, 0},
		{"no answers", nil, 0, 1800, 0},
		{"minimum override", []*RR{cname(10)}, 60, 1800, 60},
		{"maximum override", []*RR{cname(300)}, 0, 60, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{config: &Config{MinTTL: tt.minTTL, MaxTTL: tt.maxTTL}, cache: NewLruCache(10)}
			resp := NewMessage(1, ResponseMsg)
			resp.SetQuestion(&Question{QNAME: "alias.example", QTYPE: QTypeA, QCLASS: ClassINET})
			resp.Answers = tt.answers
			wire, err := resp.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			// A refresh without a cache lifetime must also discard a stale entry.
			c.cache.Set(qKey(resp.Question), []byte("stale response"), 60)
			before := time.Now().Unix()
			if err := c.handleAnswer(wire, "fixture", "fixture", "udp", "DIRECT"); err != nil {
				t.Fatal(err)
			}
			after := time.Now().Unix()
			key := qKey(resp.Question)
			entry, ok := c.cache.cache[key]
			if tt.wantTTL == 0 {
				if ok {
					t.Fatalf("response with no positive TTL was cached until %d", entry.exp)
				}
				if _, ok := c.cache.store[key]; ok {
					t.Fatal("response was cached permanently")
				}
				return
			}
			if !ok {
				t.Fatal("response was not cached")
			}
			if entry.exp < before+int64(tt.wantTTL) || entry.exp > after+int64(tt.wantTTL) {
				t.Fatalf("cache expiration = %d, want %d..%d", entry.exp, before+int64(tt.wantTTL), after+int64(tt.wantTTL))
			}
			got, expired := c.cache.Get(key)
			if expired || !bytes.Equal(got, wire) {
				t.Fatalf("cached response = %x, expired = %v", got, expired)
			}
		})
	}
}

func TestUncacheableAnswersLeaveCacheUsable(t *testing.T) {
	c := &Client{config: &Config{MaxTTL: 1800}, cache: NewLruCache(2)}
	var responses [][]byte
	for _, name := range []string{"first.example", "second.example"} {
		resp := NewMessage(1, ResponseMsg)
		resp.SetQuestion(&Question{QNAME: name, QTYPE: QTypeA, QCLASS: ClassINET})
		wire, err := resp.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, wire)
		c.cache.Set(qKey(resp.Question), []byte("stale"), 60)
	}
	for _, wire := range responses {
		if err := c.handleAnswer(wire, "fixture", "fixture", "udp", "DIRECT"); err != nil {
			t.Fatal(err)
		}
	}
	c.cache.Set("next", []byte("response"), 30)
	if got, expired := c.cache.Get("next"); expired || string(got) != "response" {
		t.Fatalf("cache after removing all responses: %q, expired = %v", got, expired)
	}
}
