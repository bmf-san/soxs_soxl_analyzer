package decay

// Package decay analyses leveraged-ETF volatility decay.

import (
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
)

// Point is one row of the decay analysis.
type Point struct {
	Time           time.Time `json:"time"`
	UnderlyingCum  float64   `json:"underlying_cum"`
	TheoreticalCum float64   `json:"theoretical_cum"`
	ActualCum      float64   `json:"actual_cum"`
	DecayPct       float64   `json:"decay_pct"`
}

// Compute compares an underlying series with a leveraged one (default 3x).
func Compute(underlying, leveraged *data.Series, leverage float64) []Point {
	// align by timestamp (date-key)
	idx := map[string]float64{}
	uPrev := 0.0
	for i, b := range underlying.Bars {
		if i == 0 {
			uPrev = b.Close
			continue
		}
		ret := b.Close/uPrev - 1
		idx[b.Time.Format("2006-01-02")] = ret
		uPrev = b.Close
	}
	out := make([]Point, 0, len(leveraged.Bars))
	uCum, tCum, aCum := 1.0, 1.0, 1.0
	lPrev := 0.0
	for i, b := range leveraged.Bars {
		key := b.Time.Format("2006-01-02")
		uRet, ok := idx[key]
		if !ok {
			lPrev = b.Close
			continue
		}
		var lRet float64
		if i > 0 && lPrev != 0 {
			lRet = b.Close/lPrev - 1
		}
		uCum *= 1 + uRet
		tCum *= 1 + uRet*leverage
		aCum *= 1 + lRet
		var decay float64
		if tCum != 0 {
			decay = (aCum/tCum - 1) * 100
		}
		out = append(out, Point{
			Time:           b.Time,
			UnderlyingCum:  uCum,
			TheoreticalCum: tCum,
			ActualCum:      aCum,
			DecayPct:       decay,
		})
		lPrev = b.Close
	}
	return out
}

// DualPoint is one row of the SOXL+SOXS dual holding simulation.
type DualPoint struct {
	Time      time.Time `json:"time"`
	SOXLValue float64   `json:"soxl_value"`
	SOXSValue float64   `json:"soxs_value"`
	Total     float64   `json:"total"`
	PnLPct    float64   `json:"pnl_pct"`
}

// SimulateDual simulates equally splitting capital between SOXL and SOXS.
func SimulateDual(bull, bear *data.Series, initial float64) []DualPoint {
	// align by date
	bearByDate := map[string]int{}
	for i, b := range bear.Bars {
		bearByDate[b.Time.Format("2006-01-02")] = i
	}
	out := make([]DualPoint, 0, len(bull.Bars))
	half := initial / 2
	bullEq, bearEq := half, half
	bullPrev, bearPrev := 0.0, 0.0
	for i, b := range bull.Bars {
		key := b.Time.Format("2006-01-02")
		bj, ok := bearByDate[key]
		if !ok {
			continue
		}
		bb := bear.Bars[bj]
		if i > 0 && bullPrev != 0 && bearPrev != 0 {
			bullEq *= b.Close / bullPrev
			bearEq *= bb.Close / bearPrev
		}
		bullPrev = b.Close
		bearPrev = bb.Close
		total := bullEq + bearEq
		out = append(out, DualPoint{
			Time:      b.Time,
			SOXLValue: bullEq,
			SOXSValue: bearEq,
			Total:     total,
			PnLPct:    (total/initial - 1) * 100,
		})
	}
	return out
}
