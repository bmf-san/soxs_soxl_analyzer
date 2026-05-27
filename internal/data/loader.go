// Package data fetches OHLCV history directly from Yahoo Finance's v8 chart API.
//
// We call the upstream JSON endpoint with a desktop User-Agent. This avoids
// the auth/crumb breakage that affects older third-party Yahoo clients
// (e.g. piquette/finance-go) since Yahoo tightened access in 2024+.
package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Bar represents a single OHLCV record.
type Bar struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume int64     `json:"volume"`
}

// Series is a chronological slice of Bars for one symbol.
type Series struct {
	Symbol string `json:"symbol"`
	Bars   []Bar  `json:"bars"`
}

// CloseSlice returns the closing price slice.
func (s *Series) CloseSlice() []float64 {
	out := make([]float64, len(s.Bars))
	for i, b := range s.Bars {
		out[i] = b.Close
	}
	return out
}

type cacheEntry struct {
	series    *Series
	expiresAt time.Time
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]cacheEntry{}
	ttl     = 15 * time.Minute
	httpCli = &http.Client{Timeout: 15 * time.Second}
	// yahooBaseURL is overridable in tests via SetBaseURL.
	yahooBaseURL = "https://query1.finance.yahoo.com"
)

// SetBaseURL overrides the upstream API base URL. Intended for tests only.
func SetBaseURL(u string) { yahooBaseURL = u }

// ClearCache empties the in-memory cache. Intended for tests.
func ClearCache() {
	cacheMu.Lock()
	cache = map[string]cacheEntry{}
	cacheMu.Unlock()
}

// PeriodToRange converts a period string to a (start,end) pair.
func PeriodToRange(period string) (time.Time, time.Time) {
	end := time.Now()
	var start time.Time
	switch period {
	case "1mo":
		start = end.AddDate(0, -1, 0)
	case "3mo":
		start = end.AddDate(0, -3, 0)
	case "6mo":
		start = end.AddDate(0, -6, 0)
	case "2y":
		start = end.AddDate(-2, 0, 0)
	case "5y":
		start = end.AddDate(-5, 0, 0)
	case "max":
		start = end.AddDate(-20, 0, 0)
	default: // "1y"
		start = end.AddDate(-1, 0, 0)
	}
	return start, end
}

// yahooChartResponse maps the subset of the v8 chart API we need.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
				AdjClose []struct {
					AdjClose []*float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// LoadSeries fetches daily OHLCV for symbol within the given period.
func LoadSeries(symbol, period string) (*Series, error) {
	key := symbol + "|" + period
	cacheMu.RLock()
	if e, ok := cache[key]; ok && time.Now().Before(e.expiresAt) {
		cacheMu.RUnlock()
		return e.series, nil
	}
	cacheMu.RUnlock()

	start, end := PeriodToRange(period)
	endpoint := fmt.Sprintf(
		"%s/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d&events=history",
		yahooBaseURL, url.PathEscape(symbol), start.Unix(), end.Unix(),
	)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch %s: http %d", symbol, resp.StatusCode)
	}

	series, err := parseChartResponse(resp.Body, symbol)
	if err != nil {
		return nil, err
	}

	cacheMu.Lock()
	cache[key] = cacheEntry{series: series, expiresAt: time.Now().Add(ttl)}
	cacheMu.Unlock()
	return series, nil
}

// parseChartResponse decodes Yahoo's chart JSON into a Series. Exposed for tests.
func parseChartResponse(body io.Reader, symbol string) (*Series, error) {
	var data yahooChartResponse
	if err := json.NewDecoder(body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode %s: %w", symbol, err)
	}
	if data.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error %s: %s", data.Chart.Error.Code, data.Chart.Error.Description)
	}
	if len(data.Chart.Result) == 0 || len(data.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("%s: no data", symbol)
	}
	r := data.Chart.Result[0]
	q := r.Indicators.Quote[0]
	var adj []*float64
	if len(r.Indicators.AdjClose) > 0 {
		adj = r.Indicators.AdjClose[0].AdjClose
	}

	bars := make([]Bar, 0, len(r.Timestamp))
	for i, ts := range r.Timestamp {
		if i >= len(q.Close) || q.Close[i] == nil {
			continue
		}
		closep := *q.Close[i]
		if i < len(adj) && adj[i] != nil {
			closep = *adj[i] // prefer adjusted close
		}
		vol := int64(0)
		if i < len(q.Volume) && q.Volume[i] != nil {
			vol = *q.Volume[i]
		}
		bars = append(bars, Bar{
			Time:   time.Unix(ts, 0).UTC(),
			Open:   safeF(q.Open, i),
			High:   safeF(q.High, i),
			Low:    safeF(q.Low, i),
			Close:  closep,
			Volume: vol,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("%s: no usable bars", symbol)
	}
	return &Series{Symbol: symbol, Bars: bars}, nil
}

func safeF(s []*float64, i int) float64 {
	if i >= len(s) || s[i] == nil {
		return 0
	}
	return *s[i]
}

// LoadMany fetches multiple symbols concurrently.
func LoadMany(symbols []string, period string) map[string]*Series {
	out := map[string]*Series{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := LoadSeries(sym, period)
			if err != nil || s == nil {
				return
			}
			mu.Lock()
			out[sym] = s
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// NormalizeTo100 rescales so the first value equals 100.
func NormalizeTo100(closes []float64) []float64 {
	if len(closes) == 0 || closes[0] == 0 {
		return closes
	}
	out := make([]float64, len(closes))
	base := closes[0]
	for i, v := range closes {
		out[i] = v / base * 100
	}
	return out
}
