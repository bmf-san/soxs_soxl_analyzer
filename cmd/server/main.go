// Command server runs the SOXL/SOXS analyzer web dashboard.
package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/backtest"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/decay"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/indicators"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/news"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/plan"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/signals"
)

var (
	addr    = flag.String("addr", ":8080", "listen address")
	webRoot = flag.String("web", "web", "web assets directory")
)

func main() {
	flag.Parse()

	tmplPath := filepath.Join(*webRoot, "templates", "index.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Fatalf("template parse: %v", err)
	}

	staticDir := filepath.Join(*webRoot, "static")
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = tmpl.Execute(w, nil)
	})

	mux.HandleFunc("/api/prices", handlePrices)
	mux.HandleFunc("/api/indicators", handleIndicators)
	mux.HandleFunc("/api/signal", handleSignal)
	mux.HandleFunc("/api/decay", handleDecay)
	mux.HandleFunc("/api/backtest", handleBacktest)
	mux.HandleFunc("/api/news", handleNews)
	mux.HandleFunc("/api/plan", handlePlan)

	log.Printf("🐂🐻 SOXL/SOXS analyzer listening on http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, withMiddleware(mux)))
}

// withMiddleware wraps the mux with access logging and a simple per-IP rate
// limiter (token bucket, 5 req/sec, burst 10) to avoid hammering Yahoo.
func withMiddleware(h http.Handler) http.Handler {
	return logMiddleware(rateLimitMiddleware(h))
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %v", clientIP(r), r.Method, r.URL.Path, time.Since(start))
	})
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

var (
	rlMu      sync.Mutex
	rlBuckets = map[string]*bucket{}
	rlRate    = 5.0  // tokens per second
	rlBurst   = 10.0 // max tokens
)

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// only rate-limit /api/*; static + index unaffected
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !rlAllow(ip) {
			writeErr(w, http.StatusTooManyRequests, "rate limit exceeded; please slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rlAllow(ip string) bool {
	rlMu.Lock()
	defer rlMu.Unlock()
	now := time.Now()
	b, ok := rlBuckets[ip]
	if !ok {
		rlBuckets[ip] = &bucket{tokens: rlBurst - 1, lastSeen: now}
		return true
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens = minF(rlBurst, b.tokens+elapsed*rlRate)
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		if i := strings.Index(xf, ","); i >= 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func paramOr(r *http.Request, key, def string) string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	return v
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
	tickersParam := paramOr(r, "tickers", "SOXL,SOXS,SOXX,SMH")
	period := paramOr(r, "period", "1y")
	tickers := strings.Split(tickersParam, ",")
	series := data.LoadMany(tickers, period)
	writeJSON(w, series)
}

func handleIndicators(w http.ResponseWriter, r *http.Request) {
	ticker := paramOr(r, "ticker", "SOXL")
	period := paramOr(r, "period", "1y")
	s, err := data.LoadSeries(ticker, period)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	high := make([]float64, len(s.Bars))
	low := make([]float64, len(s.Bars))
	closep := make([]float64, len(s.Bars))
	for i, b := range s.Bars {
		high[i] = b.High
		low[i] = b.Low
		closep[i] = b.Close
	}
	bundle := indicators.AttachAll(high, low, closep)
	writeJSON(w, map[string]any{
		"series":     s,
		"indicators": bundle,
	})
}

func handleSignal(w http.ResponseWriter, r *http.Request) {
	ticker := paramOr(r, "ticker", "SOXL")
	period := paramOr(r, "period", "1y")
	s, err := data.LoadSeries(ticker, period)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	closep := s.CloseSlice()
	high := make([]float64, len(s.Bars))
	low := make([]float64, len(s.Bars))
	for i, b := range s.Bars {
		high[i] = b.High
		low[i] = b.Low
	}
	bundle := indicators.AttachAll(high, low, closep)
	sig := signals.Generate(closep, bundle)
	if len(s.Bars) < 2 {
		writeErr(w, http.StatusBadGateway, "insufficient bars")
		return
	}
	last := s.Bars[len(s.Bars)-1]
	prev := s.Bars[len(s.Bars)-2]
	dayChange := 0.0
	if prev.Close != 0 {
		dayChange = (last.Close/prev.Close - 1) * 100
	}
	writeJSON(w, map[string]any{
		"ticker":     ticker,
		"close":      last.Close,
		"day_change": dayChange,
		"rsi14":      bundle.RSI14[len(bundle.RSI14)-1],
		"hv20":       bundle.HV20[len(bundle.HV20)-1],
		"signal":     sig,
	})
}

func handleDecay(w http.ResponseWriter, r *http.Request) {
	period := paramOr(r, "period", "1y")
	leverage, _ := strconv.ParseFloat(paramOr(r, "leverage", "3.0"), 64)
	capital, _ := strconv.ParseFloat(paramOr(r, "capital", "10000"), 64)

	soxx, err1 := data.LoadSeries("SOXX", period)
	soxl, err2 := data.LoadSeries("SOXL", period)
	soxs, err3 := data.LoadSeries("SOXS", period)
	if err1 != nil || err2 != nil || err3 != nil {
		writeErr(w, http.StatusBadGateway, "failed to load decay data")
		return
	}
	resp := map[string]any{
		"soxl_decay": decay.Compute(soxx, soxl, leverage),
		"soxs_decay": decay.Compute(soxx, soxs, -leverage),
		"dual":       decay.SimulateDual(soxl, soxs, capital),
	}
	writeJSON(w, resp)
}

func handleBacktest(w http.ResponseWriter, r *http.Request) {
	ticker := paramOr(r, "ticker", "SOXL")
	period := paramOr(r, "period", "1y")
	strategy := paramOr(r, "strategy", "MACD")
	capital, _ := strconv.ParseFloat(paramOr(r, "capital", "10000"), 64)
	feeBps, _ := strconv.ParseFloat(paramOr(r, "fee_bps", "5"), 64)
	s, err := data.LoadSeries(ticker, period)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	res, err := backtest.Run(s, strategy, capital, feeBps)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// also send buy-and-hold curve
	bh := make([]backtest.EquityPoint, len(s.Bars))
	base := s.Bars[0].Close
	for i, b := range s.Bars {
		bh[i] = backtest.EquityPoint{Time: b.Time, Equity: capital * b.Close / base}
	}
	writeJSON(w, map[string]any{
		"result":   res,
		"buy_hold": bh,
		"ticker":   ticker,
		"strategy": strategy,
	})
}

func handleNews(w http.ResponseWriter, r *http.Request) {
	tickersParam := paramOr(r, "tickers", "SOXL,SOXS,SMH,NVDA,AMD,TSM")
	limit, _ := strconv.Atoi(paramOr(r, "limit", "40"))
	tickers := strings.Split(tickersParam, ",")
	writeJSON(w, news.Fetch(tickers, limit))
}

func handlePlan(w http.ResponseWriter, r *http.Request) {
	capital, _ := strconv.ParseFloat(paramOr(r, "capital", "10000"), 64)
	riskPct, _ := strconv.ParseFloat(paramOr(r, "risk_pct", "1.5"), 64)
	force := strings.ToUpper(strings.TrimSpace(paramOr(r, "force", "")))
	if capital <= 0 {
		writeErr(w, http.StatusBadRequest, "capital must be > 0")
		return
	}
	if riskPct <= 0 || riskPct > 20 {
		writeErr(w, http.StatusBadRequest, "risk_pct must be in (0, 20]")
		return
	}
	switch force {
	case "", "AUTO", "SOXL", "SOXS":
	default:
		writeErr(w, http.StatusBadRequest, "force must be one of: AUTO, SOXL, SOXS")
		return
	}
	p, err := plan.Build(capital, riskPct, force)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, p)
}
