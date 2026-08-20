package coingecko

import "testing"

func TestNormalizeDecimalExpandsExactly(t *testing.T) {
	tests := []struct {
		input  string
		signed bool
		want   string
		ok     bool
	}{
		{input: "1.25e-8", want: "0.0000000125", ok: true},
		{input: "7.0187000e4", want: "70187", ok: true},
		{input: "-4.2500", signed: true, want: "-4.25", ok: true},
		{input: "-4.25", want: "", ok: false},
		{input: "NaN", want: "", ok: false},
		{input: "1e1000", want: "", ok: false},
	}
	for _, test := range tests {
		got, ok := normalizeDecimal(test.input, test.signed)
		if got != test.want || ok != test.ok {
			t.Fatalf("normalizeDecimal(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}
