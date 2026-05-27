package signals

// Package signals scores bull/bear bias from technical indicators.

import (
	"math"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/indicators"
)

// Result is the latest scored signal.
type Result struct {
	Score   float64            `json:"score"`
	Label   string             `json:"label"`
	Details map[string]float64 `json:"details"`
}

func scoreRSI(v float64) float64 {
	switch {
	case v < 30:
		return 1.0
	case v < 45:
		return 0.5
	case v > 70:
		return -1.0
	case v > 55:
		return -0.5
	}
	return 0
}

func scoreMACD(macd, sig, hist float64) float64 {
	if macd > sig && hist > 0 {
		return 1.0
	}
	if macd < sig && hist < 0 {
		return -1.0
	}
	return 0
}

func scoreMA(closep, sma20, sma50, sma200 float64) float64 {
	// Treat exact equality as neutral (0) so a perfectly flat market does not
	// register as bearish via the else-branches.
	score := 0.0
	switch {
	case closep > sma20:
		score += 0.33
	case closep < sma20:
		score -= 0.33
	}
	switch {
	case sma20 > sma50:
		score += 0.33
	case sma20 < sma50:
		score -= 0.33
	}
	switch {
	case sma50 > sma200:
		score += 0.34
	case sma50 < sma200:
		score -= 0.34
	}
	return score
}

func scoreBB(closep, upper, lower float64) float64 {
	width := upper - lower
	if width <= 0 {
		return 0
	}
	pos := (closep - lower) / width
	switch {
	case pos < 0.1:
		return 1.0
	case pos > 0.9:
		return -1.0
	}
	return (0.5 - pos) * 2 * 0.5
}

func last(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	v := s[len(s)-1]
	if math.IsNaN(v) {
		return 0
	}
	return v
}

// Generate computes a -1..+1 bullish/bearish score using the latest values.
func Generate(closep []float64, b indicators.Bundle) Result {
	c := last(closep)
	sRSI := scoreRSI(last(b.RSI14))
	sMACD := scoreMACD(last(b.MACD.MACD), last(b.MACD.Signal), last(b.MACD.Hist))
	sma200 := last(b.SMA200)
	if sma200 == 0 {
		sma200 = last(b.SMA50)
	}
	sMA := scoreMA(c, last(b.SMA20), last(b.SMA50), sma200)
	sBB := scoreBB(c, last(b.BB.Upper), last(b.BB.Lower))

	weights := map[string]float64{"RSI": 0.25, "MACD": 0.30, "MA": 0.30, "BB": 0.15}
	components := map[string]float64{"RSI": sRSI, "MACD": sMACD, "MA": sMA, "BB": sBB}
	score := 0.0
	for k, w := range weights {
		score += components[k] * w
	}
	score = math.Max(-1, math.Min(1, score))

	var label string
	switch {
	case score >= 0.5:
		label = "🐂 強気買い (SOXL 推奨)"
	case score >= 0.2:
		label = "🐂 弱気買い (SOXL 寄り)"
	case score <= -0.5:
		label = "🐻 強気売り (SOXS 推奨)"
	case score <= -0.2:
		label = "🐻 弱気売り (SOXS 寄り)"
	default:
		label = "⚪️ 中立(様子見)"
	}
	return Result{Score: round3(score), Label: label, Details: components}
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
