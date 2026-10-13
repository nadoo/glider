package dns

import (
	"fmt"
	"net/netip"
	"testing"
)

func TestUnmarshalAddressRecordLength(t *testing.T) {
	for _, tt := range []struct {
		qtype uint16
		size  int
		valid bool
	}{
		{QTypeA, 0, false},
		{QTypeA, 1, false},
		{QTypeA, 3, false},
		{QTypeA, 4, true},
		{QTypeA, 5, false},
		{QTypeAAAA, 0, false},
		{QTypeAAAA, 1, false},
		{QTypeAAAA, 15, false},
		{QTypeAAAA, 16, true},
		{QTypeAAAA, 17, false},
	} {
		t.Run(fmt.Sprintf("type_%d_length_%d", tt.qtype, tt.size), func(t *testing.T) {
			defer func() {
				if value := recover(); value != nil {
					t.Errorf("UnmarshalMessage panicked: %v", value)
				}
			}()
			data := make([]byte, tt.size)
			for i := range data {
				data[i] = byte(i + 1)
			}
			message := NewMessage(1, ResponseMsg)
			message.SetQuestion(NewQuestion(tt.qtype, "example.com"))
			message.AddAnswer(&RR{NAME: "example.com", TYPE: tt.qtype, CLASS: ClassINET, TTL: 60, RDLENGTH: uint16(len(data)), RDATA: data})
			wire, err := message.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := UnmarshalMessage(wire[:len(wire):len(wire)])
			if !tt.valid {
				if err == nil {
					t.Fatal("accepted an invalid address record length")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, _ := netip.AddrFromSlice(data)
			if len(parsed.Answers) != 1 || parsed.Answers[0].IP != want {
				t.Fatalf("decoded address = %v, want %s", parsed.Answers, want)
			}
		})
	}
}
