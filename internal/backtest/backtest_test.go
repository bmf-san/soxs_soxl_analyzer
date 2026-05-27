package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
)

func makeSeries(closes []float64) *data.Series {
	bars := make([]data.Bar, len(closes))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		bars[i] = data.Bar{
			Time:  base.AddDate(0, 0, i),
			Open:  c, High: c + 0.5, Low: c - 0.5, Close: c,
		}
	}
	return &data.Series{Symbol: "TEST", Bars: bars}
}

func TestRun_TooFewBars(t *testing.T) {
	s := makeSeries(make([]float64, 30))
	for i := range s.Bars {
		s.Bars[i].Close = 100
	}
	_, err := Run(s, "MACD", 10000, 0)
	if err == nil {
		t.Fatal("expected error for short series")
	}
}

func TestRun_UnknownStrategy(t *testing.T) {
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	_, err := Run(makeSeries(closes), "UNKNOWN", 10000, 0)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

// Verify equity is non-decreasing for an always-up MA_CROSS scenario.
func TestRun_MACross_Uptrend(t *testing.T) {
	closes := make([]float64, 250)
	for i := range closes {
		closes[i] = 100 + float64(i)*0.5
	}
	res, err := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats["total_return_pct"] <= 0 {
		t.Fatalf("expected positive return on uptrend, got %v", res.Stats["total_return_pct"])
	}
}

// Critical: Trade EntryPrice * (1+ReturnPct/100) ≈ ExitPrice. This is the bug we fixed.
func TestRun_TradeReturnConsistency(t *testing.T) {
	// Create alternating regime: 50 bars up, 50 bars down, 50 bars up
	closes := make([]float64, 200)
	closes[0] = 100
	for i := 1; i < 50; i++ {
		closes[i] = closes[i-1] * 1.01
	}
	for i := 50; i < 120; i++ {
		closes[i] = closes[i-1] * 0.99
	}
	for i := 120; i < 200; i++ {
		closes[i] = closes[i-1] * 1.01
	}
	res, err := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trades) == 0 {
		t.Fatal("expected at least one trade")
	}
	for i, tr := range res.Trades {
		impliedExit := tr.EntryPrice * (1 + tr.ReturnPct/100)
		if math.Abs(impliedExit-tr.ExitPrice) > 1e-6 {
			t.Errorf("trade %d: entry=%v exit=%v ret=%v%% but implied exit=%v",
				i, tr.EntryPrice, tr.ExitPrice, tr.ReturnPct, impliedExit)
		}
		if !tr.Exit.After(tr.Entry) && !tr.Exit.Equal(tr.Entry) {
			t.Errorf("trade %d: exit %v before entry %v", i, tr.Exit, tr.Entry)
		}
	}
}

// Verify product of (1+trade returns) approximately matches equity total return
// (in absence of fees, and assuming no concurrent overlap which long-only ensures).
func TestRun_TradesMatchEquity(t *testing.T) {
	closes := make([]float64, 200)
	closes[0] = 100
	for i := 1; i < 100; i++ {
		closes[i] = closes[i-1] * 1.005
	}
	for i := 100; i < 200; i++ {
		closes[i] = closes[i-1] * 0.995
	}
	res, err := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	product := 1.0
	for _, tr := range res.Trades {
		product *= 1 + tr.ReturnPct/100
	}
	equityRet := res.Stats["total_return_pct"] / 100
	// allow 0.5% slack for rounding (stats round2)
	if math.Abs(product-(1+equityRet)) > 0.01 {
		t.Errorf("trade product=%v equity factor=%v", product, 1+equityRet)
	}
}

func TestRun_FeesReduceEquity(t *testing.T) {
	closes := make([]float64, 200)
	closes[0] = 100
	// oscillating series → many trades
	for i := 1; i < 200; i++ {
		if (i/10)%2 == 0 {
			closes[i] = closes[i-1] * 1.01
		} else {
			closes[i] = closes[i-1] * 0.99
		}
	}
	noFee, _ := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	withFee, _ := Run(makeSeries(closes), "MA_CROSS", 10000, 50) // 50 bps
	if withFee.Stats["total_return_pct"] >= noFee.Stats["total_return_pct"] {
		t.Errorf("fees did not reduce return: no_fee=%v with_fee=%v",
			noFee.Stats["total_return_pct"], withFee.Stats["total_return_pct"])
	}
}

func TestRun_StatsBounds(t *testing.T) {
	closes := make([]float64, 200)
	closes[0] = 100
	for i := 1; i < 200; i++ {
		closes[i] = closes[i-1] * (1 + math.Sin(float64(i)/7)*0.01)
	}
	res, err := Run(makeSeries(closes), "MACD", 10000, 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats["max_dd_pct"] > 0 {
		t.Errorf("max_dd should be ≤0, got %v", res.Stats["max_dd_pct"])
	}
	if res.Stats["win_rate_pct"] < 0 || res.Stats["win_rate_pct"] > 100 {
		t.Errorf("win_rate out of range: %v", res.Stats["win_rate_pct"])
	}
	if res.Stats["profit_factor"] < 0 {
		t.Errorf("profit factor negative: %v", res.Stats["profit_factor"])
	}
}

func TestGeneratePositions_MA_CROSS(t *testing.T) {
	// uptrend → eventually pos=1
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	pos, err := generatePositions(closes, "MA_CROSS")
	if err != nil {
		t.Fatal(err)
	}
	// after the 50-bar slow SMA fills, fast > slow should hold → last few are 1
	if pos[len(pos)-1] != 1 {
		t.Fatalf("expected long at end of uptrend, got %d", pos[len(pos)-1])
	}
}

func TestRun_OpenPositionAtEnd(t *testing.T) {
	// Pure uptrend → MA_CROSS will go long and stay long. Open trade should be
	// recorded at end so trades list reconciles with equity.
	closes := make([]float64, 200)
	for i := range closes {
		closes[i] = 100 + float64(i)*0.5
	}
	res, err := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trades) == 0 {
		t.Fatal("expected an open-position trade at end")
	}
}
