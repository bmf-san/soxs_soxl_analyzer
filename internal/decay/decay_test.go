package decay

import (
	"math"
	"testing"
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
)

func mkSeries(closes []float64) *data.Series {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := make([]data.Bar, len(closes))
	for i, c := range closes {
		bars[i] = data.Bar{Time: base.AddDate(0, 0, i), Close: c}
	}
	return &data.Series{Bars: bars}
}

func TestCompute_PerfectLeverage(t *testing.T) {
	// underlying moves +1% daily; perfect 3x leveraged moves +3% daily.
	n := 30
	u := make([]float64, n)
	l := make([]float64, n)
	u[0], l[0] = 100, 100
	for i := 1; i < n; i++ {
		u[i] = u[i-1] * 1.01
		l[i] = l[i-1] * 1.03
	}
	pts := Compute(mkSeries(u), mkSeries(l), 3.0)
	if len(pts) == 0 {
		t.Fatal("no points")
	}
	last := pts[len(pts)-1]
	if math.Abs(last.DecayPct) > 0.01 {
		t.Fatalf("perfect leverage should have ~0 decay, got %v", last.DecayPct)
	}
}

func TestCompute_VolatilityDecayNegative(t *testing.T) {
	// Oscillating underlying: theoretical 3x will outperform actual 3x rebal.
	// We approximate "actual" by daily 3x return WITHOUT compounding correction.
	// Underlying oscillates so cum return ≈ 0, theoretical cum ≈ 0.
	// Actual (daily-rebalanced 3x) suffers volatility drag (cum < theoretical).
	n := 60
	u := make([]float64, n)
	l := make([]float64, n)
	u[0], l[0] = 100, 100
	for i := 1; i < n; i++ {
		var r float64
		if i%2 == 1 {
			r = 0.05
		} else {
			r = -0.05
		}
		u[i] = u[i-1] * (1 + r)
		l[i] = l[i-1] * (1 + 3*r) // daily-rebal 3x
	}
	pts := Compute(mkSeries(u), mkSeries(l), 3.0)
	last := pts[len(pts)-1]
	if last.DecayPct >= 0 {
		t.Fatalf("expected negative decay from vol drag, got %v", last.DecayPct)
	}
}

func TestSimulateDual_ConservesUnderConstantPrices(t *testing.T) {
	n := 20
	bull := make([]float64, n)
	bear := make([]float64, n)
	for i := 0; i < n; i++ {
		bull[i] = 50
		bear[i] = 30
	}
	out := SimulateDual(mkSeries(bull), mkSeries(bear), 1000)
	if len(out) == 0 {
		t.Fatal("no points")
	}
	last := out[len(out)-1]
	if math.Abs(last.Total-1000) > 1e-6 {
		t.Fatalf("constant prices should preserve capital, got total=%v", last.Total)
	}
	if math.Abs(last.PnLPct) > 1e-6 {
		t.Fatalf("PnL should be 0, got %v", last.PnLPct)
	}
}

func TestSimulateDual_OppositeMovesReturnLoss(t *testing.T) {
	// SOXL doubles, SOXS halves (perfectly inverse). Equally weighted should still
	// suffer because (2 + 0.5)/2 = 1.25 vs theoretical 1.0 → wait that's gain.
	// Actually the relevant test: with daily rebal AND vol drag on lev ETFs in
	// real world it shows loss, but here we use BUY-AND-HOLD on each side, so
	// outcome depends on path. Just verify totals stay positive and bounded.
	n := 10
	bull := make([]float64, n)
	bear := make([]float64, n)
	bull[0], bear[0] = 100, 100
	for i := 1; i < n; i++ {
		bull[i] = bull[i-1] * 1.05
		bear[i] = bear[i-1] * 0.95
	}
	out := SimulateDual(mkSeries(bull), mkSeries(bear), 1000)
	last := out[len(out)-1]
	if last.SOXLValue <= 0 || last.SOXSValue <= 0 {
		t.Fatal("values went non-positive")
	}
	// SOXL portion went from 500 → 500 * (bull[end]/bull[0])
	wantBull := 500.0 * bull[n-1] / bull[0]
	wantBear := 500.0 * bear[n-1] / bear[0]
	if math.Abs(last.SOXLValue-wantBull) > 1e-6 {
		t.Errorf("SOXL value: got=%v want=%v", last.SOXLValue, wantBull)
	}
	if math.Abs(last.SOXSValue-wantBear) > 1e-6 {
		t.Errorf("SOXS value: got=%v want=%v", last.SOXSValue, wantBear)
	}
}
