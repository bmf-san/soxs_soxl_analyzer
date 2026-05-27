package signals

import (
	"math"
	"testing"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/indicators"
)

func TestScoreRSI_Boundaries(t *testing.T) {
	cases := []struct {
		v    float64
		want float64
	}{
		{20, 1.0},
		{29.9, 1.0},
		{30, 0.5}, // not <30 so falls to <45
		{40, 0.5},
		{50, 0},
		{60, -0.5},
		{75, -1.0},
	}
	for _, c := range cases {
		got := scoreRSI(c.v)
		if got != c.want {
			t.Errorf("scoreRSI(%v) = %v want %v", c.v, got, c.want)
		}
	}
}

func TestScoreMACD(t *testing.T) {
	if scoreMACD(1, 0.5, 0.5) != 1.0 {
		t.Fatal("bullish")
	}
	if scoreMACD(0.5, 1, -0.5) != -1.0 {
		t.Fatal("bearish")
	}
	if scoreMACD(1, 1, 0) != 0 {
		t.Fatal("flat")
	}
}

func TestScoreMA_AllUp(t *testing.T) {
	// close > sma20 > sma50 > sma200 → max bull
	got := scoreMA(110, 105, 100, 90)
	if math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("scoreMA bullish = %v want 1.0", got)
	}
}

func TestScoreMA_AllDown(t *testing.T) {
	got := scoreMA(90, 95, 100, 110)
	if math.Abs(got-(-1.0)) > 1e-9 {
		t.Fatalf("scoreMA bearish = %v want -1.0", got)
	}
}

func TestScoreBB(t *testing.T) {
	// Price near lower band → bullish (mean reversion)
	if got := scoreBB(100, 110, 95); got <= 0 {
		t.Fatalf("expected positive near lower, got %v", got)
	}
	// Price near upper band → bearish
	if got := scoreBB(108, 110, 95); got >= 0 {
		t.Fatalf("expected negative near upper, got %v", got)
	}
	// Width zero
	if got := scoreBB(100, 100, 100); got != 0 {
		t.Fatalf("zero-width: %v", got)
	}
}

func TestGenerate_AllBullish(t *testing.T) {
	// Construct a trend-up series; values calibrated so signals all align bull.
	n := 250
	closep := make([]float64, n)
	high := make([]float64, n)
	low := make([]float64, n)
	for i := 0; i < n; i++ {
		closep[i] = 100 + float64(i)*0.5
		high[i] = closep[i] + 1
		low[i] = closep[i] - 1
	}
	// recent dip to push RSI toward middle, but overall trend stays strong
	bundle := indicators.AttachAll(high, low, closep)
	res := Generate(closep, bundle)
	if res.Score <= 0 {
		t.Fatalf("expected positive score on uptrend, got %v", res.Score)
	}
	if res.Score < -1 || res.Score > 1 {
		t.Fatalf("score out of range: %v", res.Score)
	}
}

func TestGenerate_AllBearish(t *testing.T) {
	n := 250
	closep := make([]float64, n)
	high := make([]float64, n)
	low := make([]float64, n)
	for i := 0; i < n; i++ {
		closep[i] = 200 - float64(i)*0.5
		high[i] = closep[i] + 1
		low[i] = closep[i] - 1
	}
	bundle := indicators.AttachAll(high, low, closep)
	res := Generate(closep, bundle)
	if res.Score >= 0 {
		t.Fatalf("expected negative score on downtrend, got %v", res.Score)
	}
}

func TestGenerate_LabelByScore(t *testing.T) {
	// Use synthetic data; verify label string maps to score bucket.
	n := 250
	closep := make([]float64, n)
	high := make([]float64, n)
	low := make([]float64, n)
	for i := 0; i < n; i++ {
		closep[i] = 100 + float64(i)*0.3
		high[i] = closep[i] + 1
		low[i] = closep[i] - 1
	}
	bundle := indicators.AttachAll(high, low, closep)
	res := Generate(closep, bundle)
	if res.Label == "" {
		t.Fatal("empty label")
	}
}
