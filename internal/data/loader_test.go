package data

import (
	"testing"
	"time"
)

func TestPeriodToRange(t *testing.T) {
	cases := []struct {
		period string
		months int
	}{
		{"1mo", 1}, {"3mo", 3}, {"6mo", 6}, {"1y", 12}, {"2y", 24}, {"5y", 60},
	}
	for _, c := range cases {
		start, end := PeriodToRange(c.period)
		days := end.Sub(start).Hours() / 24
		minDays := float64(c.months)*28 - 2
		maxDays := float64(c.months)*31 + 2
		if days < minDays || days > maxDays {
			t.Errorf("period=%s days=%v not in [%v,%v]", c.period, days, minDays, maxDays)
		}
	}
}

func TestPeriodToRange_Default(t *testing.T) {
	start, end := PeriodToRange("garbage")
	if end.Sub(start) <= 0 {
		t.Fatal("end<=start")
	}
	// default should be 1y
	days := end.Sub(start).Hours() / 24
	if days < 360 || days > 370 {
		t.Errorf("default not 1y: %v days", days)
	}
}

func TestSeries_CloseSlice(t *testing.T) {
	s := &Series{Bars: []Bar{
		{Time: time.Now(), Close: 1},
		{Time: time.Now(), Close: 2},
		{Time: time.Now(), Close: 3},
	}}
	cs := s.CloseSlice()
	if len(cs) != 3 || cs[0] != 1 || cs[2] != 3 {
		t.Fatalf("CloseSlice: %v", cs)
	}
}

func TestNormalizeTo100(t *testing.T) {
	in := []float64{50, 75, 100, 25}
	out := NormalizeTo100(in)
	if out[0] != 100 {
		t.Errorf("first not 100: %v", out[0])
	}
	if out[1] != 150 || out[2] != 200 || out[3] != 50 {
		t.Errorf("normalize: %v", out)
	}
}

func TestNormalizeTo100_Empty(t *testing.T) {
	out := NormalizeTo100([]float64{})
	if len(out) != 0 {
		t.Fatal("non-empty result for empty input")
	}
}

func TestNormalizeTo100_ZeroBase(t *testing.T) {
	in := []float64{0, 1, 2}
	out := NormalizeTo100(in)
	// returns input as-is when base is 0
	if len(out) != 3 || out[0] != 0 {
		t.Fatalf("zero-base: %v", out)
	}
}
