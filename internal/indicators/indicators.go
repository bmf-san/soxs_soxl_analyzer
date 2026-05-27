package indicators

// Package indicators computes technical indicators with pure Go.

import "math"

// SMA computes a simple moving average. Result has same length as src;
// positions before window-1 fall back to a partial mean.
func SMA(src []float64, window int) []float64 {
	out := make([]float64, len(src))
	if window <= 0 {
		return out
	}
	sum := 0.0
	for i, v := range src {
		sum += v
		if i >= window {
			sum -= src[i-window]
		}
		denom := float64(window)
		if i+1 < window {
			denom = float64(i + 1)
		}
		out[i] = sum / denom
	}
	return out
}

// EMA computes an exponential moving average (adjust=false style).
func EMA(src []float64, window int) []float64 {
	out := make([]float64, len(src))
	if len(src) == 0 || window <= 0 {
		return out
	}
	alpha := 2.0 / (float64(window) + 1.0)
	out[0] = src[0]
	for i := 1; i < len(src); i++ {
		out[i] = alpha*src[i] + (1-alpha)*out[i-1]
	}
	return out
}

// RSI implements Wilder's RSI.
func RSI(src []float64, window int) []float64 {
	out := make([]float64, len(src))
	if len(src) < 2 || window <= 0 {
		for i := range out {
			out[i] = 50
		}
		return out
	}
	alpha := 1.0 / float64(window)
	var avgGain, avgLoss float64
	out[0] = 50
	for i := 1; i < len(src); i++ {
		d := src[i] - src[i-1]
		gain := 0.0
		loss := 0.0
		if d > 0 {
			gain = d
		} else {
			loss = -d
		}
		if i == 1 {
			avgGain = gain
			avgLoss = loss
		} else {
			avgGain = alpha*gain + (1-alpha)*avgGain
			avgLoss = alpha*loss + (1-alpha)*avgLoss
		}
		switch {
		case avgGain == 0 && avgLoss == 0:
			// No movement at all → neutral (Wilder's RSI is undefined here;
			// returning 100 would falsely signal extreme overbought).
			out[i] = 50
		case avgLoss == 0:
			out[i] = 100
		default:
			rs := avgGain / avgLoss
			out[i] = 100 - 100/(1+rs)
		}
	}
	return out
}

// MACDResult bundles the three MACD series.
type MACDResult struct {
	MACD   []float64 `json:"macd"`
	Signal []float64 `json:"signal"`
	Hist   []float64 `json:"hist"`
}

// MACD computes MACD line, signal line, and histogram.
func MACD(src []float64, fast, slow, signal int) MACDResult {
	emaFast := EMA(src, fast)
	emaSlow := EMA(src, slow)
	macd := make([]float64, len(src))
	for i := range src {
		macd[i] = emaFast[i] - emaSlow[i]
	}
	sig := EMA(macd, signal)
	hist := make([]float64, len(src))
	for i := range src {
		hist[i] = macd[i] - sig[i]
	}
	return MACDResult{MACD: macd, Signal: sig, Hist: hist}
}

// BollingerResult contains upper, mid (SMA), lower bands.
type BollingerResult struct {
	Upper []float64 `json:"upper"`
	Mid   []float64 `json:"mid"`
	Lower []float64 `json:"lower"`
}

// Bollinger computes Bollinger Bands.
func Bollinger(src []float64, window int, numStd float64) BollingerResult {
	mid := SMA(src, window)
	upper := make([]float64, len(src))
	lower := make([]float64, len(src))
	for i := range src {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		// std dev over [start..i]
		n := float64(i - start + 1)
		mean := mid[i]
		ss := 0.0
		for j := start; j <= i; j++ {
			d := src[j] - mean
			ss += d * d
		}
		std := math.Sqrt(ss / n)
		upper[i] = mid[i] + numStd*std
		lower[i] = mid[i] - numStd*std
	}
	return BollingerResult{Upper: upper, Mid: mid, Lower: lower}
}

// ATR computes the Average True Range from OHLC slices.
func ATR(high, low, closep []float64, window int) []float64 {
	n := len(closep)
	out := make([]float64, n)
	if n == 0 || window <= 0 {
		return out
	}
	tr := make([]float64, n)
	for i := 0; i < n; i++ {
		if i == 0 {
			tr[i] = high[i] - low[i]
			continue
		}
		a := high[i] - low[i]
		b := math.Abs(high[i] - closep[i-1])
		c := math.Abs(low[i] - closep[i-1])
		tr[i] = math.Max(a, math.Max(b, c))
	}
	return EMA(tr, window)
}

// HVAnnualized returns rolling annualized volatility (%, 252 trading days).
func HVAnnualized(closep []float64, window int) []float64 {
	n := len(closep)
	out := make([]float64, n)
	if n < 2 {
		return out
	}
	rets := make([]float64, n)
	for i := 1; i < n; i++ {
		if closep[i-1] == 0 {
			continue
		}
		rets[i] = closep[i]/closep[i-1] - 1
	}
	for i := 0; i < n; i++ {
		start := i - window + 1
		if start < 1 {
			start = 1
		}
		count := i - start + 1
		if count < 2 {
			continue
		}
		mean := 0.0
		for j := start; j <= i; j++ {
			mean += rets[j]
		}
		mean /= float64(count)
		ss := 0.0
		for j := start; j <= i; j++ {
			d := rets[j] - mean
			ss += d * d
		}
		std := math.Sqrt(ss / float64(count-1))
		out[i] = std * math.Sqrt(252) * 100
	}
	return out
}

// Bundle is a convenience struct returned to the frontend.
type Bundle struct {
	SMA20  []float64       `json:"sma20"`
	SMA50  []float64       `json:"sma50"`
	SMA200 []float64       `json:"sma200"`
	EMA20  []float64       `json:"ema20"`
	RSI14  []float64       `json:"rsi14"`
	MACD   MACDResult      `json:"macd"`
	BB     BollingerResult `json:"bb"`
	ATR14  []float64       `json:"atr14"`
	HV20   []float64       `json:"hv20"`
}

// AttachAll computes a standard set of indicators.
func AttachAll(high, low, closep []float64) Bundle {
	return Bundle{
		SMA20:  SMA(closep, 20),
		SMA50:  SMA(closep, 50),
		SMA200: SMA(closep, 200),
		EMA20:  EMA(closep, 20),
		RSI14:  RSI(closep, 14),
		MACD:   MACD(closep, 12, 26, 9),
		BB:     Bollinger(closep, 20, 2.0),
		ATR14:  ATR(high, low, closep, 14),
		HV20:   HVAnnualized(closep, 20),
	}
}
