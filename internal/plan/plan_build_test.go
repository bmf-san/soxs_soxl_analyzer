package plan

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
)

// makeSeries creates a synthetic Series with a geometric trend.
// Note: indicator signals on smooth synthetic data don't map cleanly to
// "bullish" or "bearish" scores (RSI saturates, BB flips, etc), so we use
// makeSeries primarily to make Build() runnable. Tests assert plumbing,
// not the auto-direction logic (which is covered by signals_test.go).
func makeSeries(trend float64) *data.Series {
	n := 200
	bars := make([]data.Bar, n)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	price := 100.0
	for i := 0; i < n; i++ {
		price *= 1 + trend
		bars[i] = data.Bar{
			Time: base.AddDate(0, 0, i),
			Open: price, High: price * 1.01, Low: price * 0.99, Close: price,
		}
	}
	return &data.Series{Symbol: "SOXL", Bars: bars}
}

func withLoader(loader func(string, string) (*data.Series, error), fn func()) {
	orig := loadSeries
	loadSeries = loader
	defer func() { loadSeries = orig }()
	fn()
}

func TestBuild_ForceSOXL_ProducesBuySOXL(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.5, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		if p.Action != "BUY_SOXL" {
			t.Errorf("force SOXL should yield BUY_SOXL, got %s", p.Action)
		}
		if p.Ticker != "SOXL" {
			t.Errorf("ticker=%s", p.Ticker)
		}
		if p.StopLoss >= p.EntryPrice {
			t.Errorf("stop %v should be below entry %v", p.StopLoss, p.EntryPrice)
		}
		if p.TakeProfit1 <= p.EntryPrice {
			t.Errorf("TP1 %v should be above entry %v", p.TakeProfit1, p.EntryPrice)
		}
	})
}

func TestBuild_ForceSOXS_ProducesBuySOXS(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		if sym == "SOXS" {
			return makeSeries(0.002), nil
		}
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.5, "SOXS")
		if err != nil {
			t.Fatal(err)
		}
		if p.Action != "BUY_SOXS" {
			t.Fatalf("force SOXS should yield BUY_SOXS, got %s", p.Action)
		}
		if p.Ticker != "SOXS" {
			t.Errorf("ticker=%s", p.Ticker)
		}
	})
}

func TestBuild_LoaderError_Propagates(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return nil, errors.New("network down")
	}, func() {
		_, err := Build(10000, 1.5, "")
		if err == nil {
			t.Fatal("expected loader error to propagate")
		}
		if !strings.Contains(err.Error(), "network down") {
			t.Errorf("expected wrapped error to mention 'network down', got %v", err)
		}
	})
}

func TestBuild_LoaderErrorOnSOXS_Propagates(t *testing.T) {
	// SOXL load OK, SOXS load fails → should error when forcing SOXS
	withLoader(func(sym, _ string) (*data.Series, error) {
		if sym == "SOXS" {
			return nil, errors.New("no SOXS data")
		}
		return makeSeries(0.005), nil
	}, func() {
		_, err := Build(10000, 1.5, "SOXS")
		if err == nil {
			t.Fatal("expected SOXS loader error to propagate")
		}
	})
}

func TestBuild_RiskSizing_RespectsCapital(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.0, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		// risk_usd should be ≤ ~1% of capital (allow rounding slack)
		if p.RiskUSD > 120 {
			t.Errorf("risk %v exceeds 1%% of capital", p.RiskUSD)
		}
		if p.PositionUSD > 10000+0.01 {
			t.Errorf("position %v exceeds capital", p.PositionUSD)
		}
		if p.Shares < 0 {
			t.Errorf("negative shares: %d", p.Shares)
		}
	})
}

func TestBuild_ZeroRiskDefaultsTo15(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 0, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		if math.IsNaN(p.RiskPctOfTotal) {
			t.Error("risk pct NaN")
		}
		// 1.5% of $10k ≈ $150
		if p.RiskUSD > 200 {
			t.Errorf("zero riskPct should default to 1.5%%, got risk_usd=%v", p.RiskUSD)
		}
	})
}

func TestBuild_ReasoningAndWarningsPopulated(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.5, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Reasoning) == 0 {
			t.Error("reasoning empty")
		}
		if len(p.Warnings) == 0 {
			t.Error("warnings empty (should always contain leverage-ETF disclaimer)")
		}
	})
}

func TestBuild_PriceOrdering(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.5, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		if !(p.TakeProfit2 >= p.TakeProfit1 && p.TakeProfit1 > p.EntryPrice && p.EntryPrice > p.StopLoss) {
			t.Errorf("price ordering violated: stop=%v entry=%v tp1=%v tp2=%v",
				p.StopLoss, p.EntryPrice, p.TakeProfit1, p.TakeProfit2)
		}
	})
}

func TestBuild_RiskRewardRatio_Positive(t *testing.T) {
	withLoader(func(sym, _ string) (*data.Series, error) {
		return makeSeries(0.005), nil
	}, func() {
		p, err := Build(10000, 1.5, "SOXL")
		if err != nil {
			t.Fatal(err)
		}
		if p.RiskRewardRatio <= 0 {
			t.Errorf("R/R should be positive, got %v", p.RiskRewardRatio)
		}
	})
}
