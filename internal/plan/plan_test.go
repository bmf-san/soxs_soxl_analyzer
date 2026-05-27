package plan

import (
	"math"
	"testing"
)

func TestRound2(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.234, 1.23},
		{1.235, 1.24},
		{0, 0},
		{-1.555, -1.56},
	}
	for _, c := range cases {
		if got := round2(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("round2(%v) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestRound2_NaNInf(t *testing.T) {
	if round2(math.NaN()) != 0 {
		t.Error("NaN should round to 0")
	}
	if round2(math.Inf(1)) != 0 {
		t.Error("Inf should round to 0")
	}
}

func TestLastNonZero(t *testing.T) {
	if got := lastNonZero([]float64{1, 2, 3, 0, 0}); got != 3 {
		t.Errorf("got %v want 3", got)
	}
	if got := lastNonZero([]float64{0, 0, 0}); got != 0 {
		t.Errorf("all-zero: %v", got)
	}
	if got := lastNonZero([]float64{1, math.NaN(), 0}); got != 1 {
		t.Errorf("skip NaN: %v", got)
	}
	if got := lastNonZero([]float64{}); got != 0 {
		t.Errorf("empty: %v", got)
	}
}

func TestFmtf(t *testing.T) {
	if fmtf(3.14159) != "3.14" {
		t.Errorf("got %q", fmtf(3.14159))
	}
	if fmtf(0) != "0.00" {
		t.Errorf("got %q", fmtf(0))
	}
}
