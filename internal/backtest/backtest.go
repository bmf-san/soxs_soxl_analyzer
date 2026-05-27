package backtest

// Package backtest provides a minimal long-only backtester.

import (
	"fmt"
	"math"
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/indicators"
)

// Trade represents a single completed round-trip trade.
type Trade struct {
	Entry      time.Time `json:"entry"`
	Exit       time.Time `json:"exit"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	ReturnPct  float64   `json:"return_pct"`
}

// EquityPoint represents a single equity-curve datapoint.
type EquityPoint struct {
	Time   time.Time `json:"time"`
	Equity float64   `json:"equity"`
}

// Result is the full backtest output.
type Result struct {
	Equity []EquityPoint      `json:"equity"`
	Trades []Trade            `json:"trades"`
	Stats  map[string]float64 `json:"stats"`
}

// generatePositions returns 0/1 positions for each bar (long-only).
func generatePositions(closep []float64, strategy string) ([]int, error) {
	n := len(closep)
	pos := make([]int, n)
	switch strategy {
	case "MACD":
		m := indicators.MACD(closep, 12, 26, 9)
		for i := 0; i < n; i++ {
			if m.MACD[i] > m.Signal[i] {
				pos[i] = 1
			}
		}
	case "RSI":
		r := indicators.RSI(closep, 14)
		inPos := false
		for i := 0; i < n; i++ {
			if !inPos && r[i] < 30 {
				inPos = true
			} else if inPos && r[i] > 55 {
				inPos = false
			}
			if inPos {
				pos[i] = 1
			}
		}
	case "MA_CROSS":
		fast := indicators.SMA(closep, 20)
		slow := indicators.SMA(closep, 50)
		for i := 0; i < n; i++ {
			if fast[i] > slow[i] {
				pos[i] = 1
			}
		}
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}
	return pos, nil
}

// Run executes a backtest. feeBps applied on position changes.
func Run(series *data.Series, strategy string, initial, feeBps float64) (*Result, error) {
	bars := series.Bars
	n := len(bars)
	if n < 60 {
		return nil, fmt.Errorf("not enough data: %d bars", n)
	}
	closep := series.CloseSlice()
	pos, err := generatePositions(closep, strategy)
	if err != nil {
		return nil, err
	}
	// shift by 1 (execute next bar)
	shifted := make([]int, n)
	for i := 1; i < n; i++ {
		shifted[i] = pos[i-1]
	}
	pos = shifted

	feeRate := feeBps / 10000.0
	equity := make([]EquityPoint, n)
	eq := initial
	prevPos := 0
	var dailyRets []float64
	trades := []Trade{}
	var entryIdx int
	var entryPrice float64
	for i := 0; i < n; i++ {
		var ret float64
		if i > 0 && closep[i-1] != 0 {
			ret = closep[i]/closep[i-1] - 1
		}
		stratRet := float64(pos[i]) * ret
		if pos[i] != prevPos {
			stratRet -= feeRate
		}
		eq *= 1 + stratRet
		equity[i] = EquityPoint{Time: bars[i].Time, Equity: eq}
		dailyRets = append(dailyRets, stratRet)

		if pos[i] == 1 && prevPos == 0 {
			// We entered at the close of the previous bar (signal originated there,
			// execution at next-bar open ≈ prev close). The day-i return we just
			// applied above (close[i]/close[i-1]-1) is the first holding day.
			if i > 0 {
				entryIdx = i - 1
				entryPrice = closep[i-1]
			} else {
				entryIdx = i
				entryPrice = closep[i]
			}
		}
		if pos[i] == 0 && prevPos == 1 {
			// We exited at the close of the previous bar (last day we held was i-1).
			exitIdx := i - 1
			if exitIdx < 0 {
				exitIdx = 0
			}
			exitPrice := closep[exitIdx]
			trades = append(trades, Trade{
				Entry:      bars[entryIdx].Time,
				Exit:       bars[exitIdx].Time,
				EntryPrice: entryPrice,
				ExitPrice:  exitPrice,
				ReturnPct:  (exitPrice/entryPrice - 1) * 100,
			})
		}
		prevPos = pos[i]
	}

	// If we end the simulation still holding a position, record the open trade
	// closed at the last bar's close so the trade list reconciles with equity.
	if prevPos == 1 && len(closep) > 0 {
		lastIdx := len(closep) - 1
		trades = append(trades, Trade{
			Entry:      bars[entryIdx].Time,
			Exit:       bars[lastIdx].Time,
			EntryPrice: entryPrice,
			ExitPrice:  closep[lastIdx],
			ReturnPct:  (closep[lastIdx]/entryPrice - 1) * 100,
		})
	}

	stats := computeStats(equity, dailyRets, trades, initial)
	return &Result{Equity: equity, Trades: trades, Stats: stats}, nil
}

func computeStats(equity []EquityPoint, rets []float64, trades []Trade, initial float64) map[string]float64 {
	final := equity[len(equity)-1].Equity
	totalRet := final/initial - 1
	days := equity[len(equity)-1].Time.Sub(equity[0].Time).Hours() / 24
	if days < 1 {
		days = 1
	}
	cagr := math.Pow(1+totalRet, 365/days) - 1

	mean := 0.0
	for _, r := range rets {
		mean += r
	}
	if len(rets) > 0 {
		mean /= float64(len(rets))
	}
	ssq := 0.0
	for _, r := range rets {
		d := r - mean
		ssq += d * d
	}
	std := 0.0
	if len(rets) > 1 {
		std = math.Sqrt(ssq / float64(len(rets)-1))
	}
	sharpe := 0.0
	if std > 0 {
		sharpe = mean / std * math.Sqrt(252)
	}

	peak := equity[0].Equity
	maxDD := 0.0
	for _, p := range equity {
		if p.Equity > peak {
			peak = p.Equity
		}
		dd := (p.Equity - peak) / peak
		if dd < maxDD {
			maxDD = dd
		}
	}

	wins, gross_p, gross_l := 0, 0.0, 0.0
	for _, t := range trades {
		if t.ReturnPct > 0 {
			wins++
			gross_p += t.ReturnPct
		} else if t.ReturnPct < 0 {
			gross_l += -t.ReturnPct
		}
	}
	winRate := 0.0
	pf := 0.0
	if len(trades) > 0 {
		winRate = float64(wins) / float64(len(trades))
	}
	if gross_l > 0 {
		pf = gross_p / gross_l
	}

	return map[string]float64{
		"total_return_pct": round2(totalRet * 100),
		"cagr_pct":         round2(cagr * 100),
		"sharpe":           round2(sharpe),
		"max_dd_pct":       round2(maxDD * 100),
		"num_trades":       float64(len(trades)),
		"win_rate_pct":     round2(winRate * 100),
		"profit_factor":    round2(pf),
	}
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}
