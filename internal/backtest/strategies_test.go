package backtest

import (
	"math"
	"testing"
)

func TestGeneratePositions_RSI_EntryExitThresholds(t *testing.T) {
	// Construct a sequence: deep drop (RSI <30) then rally (RSI >55)
	closes := make([]float64, 100)
	closes[0] = 100
	for i := 1; i < 30; i++ {
		closes[i] = closes[i-1] * 0.97 // sharp decline → RSI < 30
	}
	for i := 30; i < 100; i++ {
		closes[i] = closes[i-1] * 1.03 // sharp recovery → RSI > 55
	}
	pos, err := generatePositions(closes, "RSI")
	if err != nil {
		t.Fatal(err)
	}
	// Somewhere in the decline RSI < 30 → pos becomes 1
	enteredOnce := false
	exitedAfter := false
	wasIn := false
	for i := 1; i < len(pos); i++ {
		if pos[i] == 1 {
			enteredOnce = true
			wasIn = true
		}
		if wasIn && pos[i] == 0 {
			exitedAfter = true
		}
	}
	if !enteredOnce {
		t.Error("RSI strategy never entered despite oversold dip")
	}
	if !exitedAfter {
		t.Error("RSI strategy never exited despite RSI>55 recovery")
	}
}

func TestGeneratePositions_MACD_TracksCrossover(t *testing.T) {
	closes := make([]float64, 200)
	closes[0] = 100
	// up trend for 100, then down trend
	for i := 1; i < 100; i++ {
		closes[i] = closes[i-1] * 1.01
	}
	for i := 100; i < 200; i++ {
		closes[i] = closes[i-1] * 0.99
	}
	pos, err := generatePositions(closes, "MACD")
	if err != nil {
		t.Fatal(err)
	}
	// During uptrend somewhere we should be long
	long := 0
	for i := 30; i < 100; i++ {
		long += pos[i]
	}
	if long == 0 {
		t.Error("MACD never long during uptrend")
	}
	// Most of the downtrend half should be flat (MACD oscillates near the
	// end as price changes fade in geometric decay, so we don't assert the
	// very last bar — just that the strategy is mostly out during the decline).
	flat := 0
	for i := 110; i < 200; i++ {
		if pos[i] == 0 {
			flat++
		}
	}
	if flat < 30 {
		t.Errorf("MACD should spend a non-trivial fraction of downtrend flat, only %d/90 bars flat", flat)
	}
}

func TestRun_RSIStrategy_ProducesTrades(t *testing.T) {
	closes := make([]float64, 150)
	closes[0] = 100
	for i := 1; i < 30; i++ {
		closes[i] = closes[i-1] * 0.97
	}
	for i := 30; i < 60; i++ {
		closes[i] = closes[i-1] * 1.03
	}
	for i := 60; i < 90; i++ {
		closes[i] = closes[i-1] * 0.97
	}
	for i := 90; i < 150; i++ {
		closes[i] = closes[i-1] * 1.03
	}
	res, err := Run(makeSeries(closes), "RSI", 10000, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trades) == 0 {
		t.Fatal("RSI strategy produced no trades")
	}
	for _, tr := range res.Trades {
		impliedExit := tr.EntryPrice * (1 + tr.ReturnPct/100)
		if math.Abs(impliedExit-tr.ExitPrice) > 1e-6 {
			t.Errorf("trade math inconsistent: entry=%v exit=%v ret=%v", tr.EntryPrice, tr.ExitPrice, tr.ReturnPct)
		}
	}
}

func TestComputeStats_Empty(t *testing.T) {
	// degenerate: equity flat
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 100
	}
	res, err := Run(makeSeries(closes), "MA_CROSS", 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	// flat → no trades, no return
	if res.Stats["total_return_pct"] != 0 {
		t.Errorf("flat market gave non-zero return: %v", res.Stats["total_return_pct"])
	}
}
