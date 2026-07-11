package reporting

import (
	"math"
	"testing"
)

func TestRoundMoney_TwoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{1.005, 1.01},
		{2.004, 2.00},
		{-1.005, -1.01},
		{0, 0},
		{0.005, 0.01},
		{-0.005, -0.01},
		{1.015, 1.02},
		{1.025, 1.03}, // half-away-from-zero (not banker's)
		{2.5, 2.50},
	}
	for _, tc := range cases {
		got := RoundMoney(tc.in)
		if got != tc.want {
			t.Fatalf("RoundMoney(%v)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestRoundMoney_NonFinite(t *testing.T) {
	if got := RoundMoney(math.NaN()); got != 0 {
		t.Fatalf("NaN -> %v want 0", got)
	}
	if got := RoundMoney(math.Inf(1)); got != 0 {
		t.Fatalf("+Inf -> %v want 0", got)
	}
	if got := RoundMoney(math.Inf(-1)); got != 0 {
		t.Fatalf("-Inf -> %v want 0", got)
	}
}
