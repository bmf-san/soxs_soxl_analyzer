package indicators

import (
	"math"
	"testing"
)

func approx(a, b, eps float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	return math.Abs(a-b) <= eps
}

func TestSMA_Constant(t *testing.T) {
	src := []float64{5, 5, 5, 5, 5}
	got := SMA(src, 3)
	for i, v := range got {
		if !approx(v, 5, 1e-9) {
			t.Fatalf("SMA constant: idx=%d got=%v want=5", i, v)
		}
	}
}

func TestSMA_PartialWindow(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5}
	got := SMA(src, 3)
	// partial means: [1], [1.5], [2], [3], [4]
	want := []float64{1, 1.5, 2, 3, 4}
	for i := range want {
		if !approx(got[i], want[i], 1e-9) {
			t.Fatalf("SMA partial: idx=%d got=%v want=%v", i, got[i], want[i])
		}
	}
}

func TestSMA_InvalidWindow(t *testing.T) {
	got := SMA([]float64{1, 2, 3}, 0)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	for _, v := range got {
		if v != 0 {
			t.Fatalf("expected zeros, got %v", got)
		}
	}
}

func TestEMA_Constant(t *testing.T) {
	src := make([]float64, 30)
	for i := range src {
		src[i] = 10
	}
	got := EMA(src, 10)
	for i, v := range got {
		if !approx(v, 10, 1e-9) {
			t.Fatalf("EMA constant idx=%d got=%v", i, v)
		}
	}
}

func TestEMA_ConvergesToTarget(t *testing.T) {
	src := make([]float64, 200)
	for i := range src {
		src[i] = 100 // step from 0 → 100 won't happen here, all 100s
	}
	src[0] = 0
	got := EMA(src, 10)
	// after many bars should be very close to 100
	last := got[len(got)-1]
	if !approx(last, 100, 1e-6) {
		t.Fatalf("EMA convergence: got=%v want≈100", last)
	}
}

func TestRSI_MonotonicallyIncreasing(t *testing.T) {
	src := make([]float64, 50)
	for i := range src {
		src[i] = float64(100 + i)
	}
	got := RSI(src, 14)
	// All gains, no losses → RSI must be 100
	last := got[len(got)-1]
	if !approx(last, 100, 1e-9) {
		t.Fatalf("RSI all-up: got=%v want=100", last)
	}
}

func TestRSI_MonotonicallyDecreasing(t *testing.T) {
	src := make([]float64, 50)
	for i := range src {
		src[i] = float64(200 - i)
	}
	got := RSI(src, 14)
	last := got[len(got)-1]
	// All losses, no gains → RSI must be 0
	if !approx(last, 0, 1e-9) {
		t.Fatalf("RSI all-down: got=%v want=0", last)
	}
}

func TestRSI_Range(t *testing.T) {
	src := []float64{
		44.34, 44.09, 44.15, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84,
		46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03, 46.41,
		46.22, 45.64, 46.21, 46.25, 45.71, 46.45, 45.78, 45.35, 44.03,
		44.18, 44.22, 44.57, 43.42, 42.66, 43.13,
	}
	got := RSI(src, 14)
	for i, v := range got {
		if v < 0 || v > 100 {
			t.Fatalf("RSI out of [0,100]: idx=%d v=%v", i, v)
		}
	}
}

func TestMACD_Lengths(t *testing.T) {
	src := make([]float64, 100)
	for i := range src {
		src[i] = float64(i) + math.Sin(float64(i)/5)*3
	}
	m := MACD(src, 12, 26, 9)
	if len(m.MACD) != 100 || len(m.Signal) != 100 || len(m.Hist) != 100 {
		t.Fatalf("MACD lengths: %d %d %d", len(m.MACD), len(m.Signal), len(m.Hist))
	}
	// Hist = MACD - Signal
	for i := range m.MACD {
		if !approx(m.Hist[i], m.MACD[i]-m.Signal[i], 1e-9) {
			t.Fatalf("MACD hist mismatch at %d", i)
		}
	}
}

func TestBollinger_BandsEnclosePrice(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	bb := Bollinger(src, 10, 2.0)
	for i := range src {
		if bb.Lower[i] > bb.Upper[i] {
			t.Fatalf("lower>upper at %d", i)
		}
		// mid must be between lower and upper
		if bb.Mid[i] < bb.Lower[i] || bb.Mid[i] > bb.Upper[i] {
			t.Fatalf("mid outside band at %d: lower=%v mid=%v upper=%v", i, bb.Lower[i], bb.Mid[i], bb.Upper[i])
		}
	}
}

func TestATR_NonNegative(t *testing.T) {
	high := []float64{10, 11, 12, 11, 10, 9, 10, 11, 12, 13, 14, 15, 14, 13, 12, 11}
	low := []float64{9, 10, 11, 10, 9, 8, 9, 10, 11, 12, 13, 14, 13, 12, 11, 10}
	closep := []float64{9.5, 10.5, 11.5, 10.5, 9.5, 8.5, 9.5, 10.5, 11.5, 12.5, 13.5, 14.5, 13.5, 12.5, 11.5, 10.5}
	atr := ATR(high, low, closep, 14)
	if len(atr) != len(closep) {
		t.Fatalf("len: %d vs %d", len(atr), len(closep))
	}
	for i, v := range atr {
		if v < 0 || math.IsNaN(v) {
			t.Fatalf("ATR negative/nan at %d: %v", i, v)
		}
	}
}

func TestHV_ZeroForConstant(t *testing.T) {
	src := make([]float64, 50)
	for i := range src {
		src[i] = 100
	}
	hv := HVAnnualized(src, 20)
	for i := 21; i < len(hv); i++ {
		if !approx(hv[i], 0, 1e-9) {
			t.Fatalf("HV constant: idx=%d got=%v want=0", i, hv[i])
		}
	}
}

func TestAttachAll_Lengths(t *testing.T) {
	n := 300
	high := make([]float64, n)
	low := make([]float64, n)
	closep := make([]float64, n)
	for i := 0; i < n; i++ {
		p := 100 + math.Sin(float64(i)/10)*5
		high[i] = p + 1
		low[i] = p - 1
		closep[i] = p
	}
	b := AttachAll(high, low, closep)
	for name, s := range map[string][]float64{
		"SMA20": b.SMA20, "SMA50": b.SMA50, "SMA200": b.SMA200,
		"EMA20": b.EMA20, "RSI14": b.RSI14, "ATR14": b.ATR14, "HV20": b.HV20,
	} {
		if len(s) != n {
			t.Fatalf("%s len=%d want=%d", name, len(s), n)
		}
	}
	if len(b.MACD.MACD) != n || len(b.BB.Mid) != n {
		t.Fatalf("MACD/BB lengths off")
	}
}
