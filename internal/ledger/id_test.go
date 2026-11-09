package ledger

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestParseID(t *testing.T) {
	tests := []struct {
		in     string
		want   string // FormatID of the parsed id; empty when parsing fails
		wantOK bool
	}{
		{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001", true},
		{"0190F3A2-7B1C-7DEF-8ABC-0123456789AB", "0190f3a2-7b1c-7def-8abc-0123456789ab", true},
		{"fffffffe-ffff-ffff-ffff-ffffffffffff", "fffffffe-ffff-ffff-ffff-ffffffffffff", true},
		{"00000000-0000-0000-0000-00000000001", "", false},
		{"00000000-0000-0000-0000-0000000000001", "", false},
		{"00000000-0000-0000-0000-00000000000g", "", false},
		{"000000000000000000000000000000000001", "", false},
		{"00000000000000000000000000000001", "", false},
		{"00000000-0000-0000-0000-000000000000", "", false},
		{"ffffffff-ffff-ffff-ffff-ffffffffffff", "", false},
		{"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF", "", false},
		{"1001", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		id, ok := parseID(tt.in)
		if ok != tt.wantOK {
			t.Errorf("parseID(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if ok && FormatID(id) != tt.want {
			t.Errorf("FormatID(parseID(%q)) = %q, want %q", tt.in, FormatID(id), tt.want)
		}
	}
}

func TestParseIDIsBigEndian(t *testing.T) {
	if id, _ := parseID("00000000-0000-0000-0000-000000000001"); id != tb.ToUint128(1) {
		t.Fatalf("parseID(...0001) = %s, want 1", id.BigInt())
	}
	// Each UUID is larger than the one before it when read big-endian.
	ordered := []string{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000100",
		"00000000-0000-0000-0001-000000000000",
		"0190f3a2-7b1c-7def-8abc-0123456789ab",
		"0190f3a2-7b1d-7000-8000-000000000000",
		"fffffffe-ffff-ffff-ffff-ffffffffffff",
	}
	for i := 1; i < len(ordered); i++ {
		a, _ := parseID(ordered[i-1])
		b, _ := parseID(ordered[i])
		if a.BigInt().Cmp(b.BigInt()) >= 0 {
			t.Errorf("parseID(%s) >= parseID(%s), want less", ordered[i-1], ordered[i])
		}
	}
}
