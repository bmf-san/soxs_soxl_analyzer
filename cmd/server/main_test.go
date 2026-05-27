package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
)

// mockYahooBody builds a valid Yahoo chart response. n bars, prices trending up.
func mockYahooBody(n int) string {
	tsStr := make([]string, n)
	closes := make([]string, n)
	vols := make([]string, n)
	for i := 0; i < n; i++ {
		tsStr[i] = fmt.Sprintf("%d", 1700000000+int64(i)*86400)
		closes[i] = fmt.Sprintf("%v", 100+float64(i)*0.5)
		vols[i] = "1000000"
	}
	return fmt.Sprintf(`{
      "chart": {
        "result": [{
          "timestamp": [%s],
          "indicators": {
            "quote": [{
              "open":[%s],"high":[%s],"low":[%s],"close":[%s],"volume":[%s]
            }],
            "adjclose":[{"adjclose":[%s]}]
          }
        }],
        "error": null
      }
    }`,
		strings.Join(tsStr, ","),
		strings.Join(closes, ","), strings.Join(closes, ","),
		strings.Join(closes, ","), strings.Join(closes, ","),
		strings.Join(vols, ","), strings.Join(closes, ","))
}

// setupMockYahoo redirects all data.* loaders to a local httptest server.
func setupMockYahoo(t *testing.T, bars int) func() {
	t.Helper()
	data.ClearCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mockYahooBody(bars))
	}))
	origBase := "https://query1.finance.yahoo.com"
	data.SetBaseURL(srv.URL)
	return func() {
		data.SetBaseURL(origBase)
		srv.Close()
		data.ClearCache()
	}
}

func TestHandlePrices_OK(t *testing.T) {
	defer setupMockYahoo(t, 300)()
	req := httptest.NewRequest("GET", "/api/prices?tickers=SOXL,SOXS&period=1y", nil)
	w := httptest.NewRecorder()
	handlePrices(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["SOXL"]; !ok {
		t.Error("SOXL missing in response")
	}
}

func TestHandleIndicators_OK(t *testing.T) {
	defer setupMockYahoo(t, 300)()
	req := httptest.NewRequest("GET", "/api/indicators?ticker=SOXL&period=1y", nil)
	w := httptest.NewRecorder()
	handleIndicators(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["indicators"]; !ok {
		t.Error("indicators missing")
	}
}

func TestHandleSignal_OK(t *testing.T) {
	defer setupMockYahoo(t, 300)()
	req := httptest.NewRequest("GET", "/api/signal?ticker=SOXL&period=1y", nil)
	w := httptest.NewRecorder()
	handleSignal(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleBacktest_OK(t *testing.T) {
	defer setupMockYahoo(t, 250)()
	req := httptest.NewRequest("GET", "/api/backtest?ticker=SOXL&strategy=MACD&period=1y&capital=10000&fee_bps=5", nil)
	w := httptest.NewRecorder()
	handleBacktest(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["buy_hold"]; !ok {
		t.Error("buy_hold missing")
	}
}

func TestHandleBacktest_BadStrategy(t *testing.T) {
	defer setupMockYahoo(t, 250)()
	req := httptest.NewRequest("GET", "/api/backtest?ticker=SOXL&strategy=NOPE", nil)
	w := httptest.NewRecorder()
	handleBacktest(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePlan_OK(t *testing.T) {
	defer setupMockYahoo(t, 200)()
	req := httptest.NewRequest("GET", "/api/plan?capital=10000&risk_pct=1.5", nil)
	w := httptest.NewRecorder()
	handlePlan(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePlan_InvalidCapital(t *testing.T) {
	cases := []string{
		"/api/plan?capital=0&risk_pct=1.5",
		"/api/plan?capital=-100&risk_pct=1.5",
	}
	for _, url := range cases {
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		handlePlan(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", url, w.Code)
		}
	}
}

func TestHandlePlan_InvalidRiskPct(t *testing.T) {
	cases := []string{
		"/api/plan?capital=10000&risk_pct=0",
		"/api/plan?capital=10000&risk_pct=-1",
		"/api/plan?capital=10000&risk_pct=25",
	}
	for _, url := range cases {
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		handlePlan(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", url, w.Code)
		}
	}
}

func TestHandlePlan_InvalidForce(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/plan?capital=10000&risk_pct=1.5&force=BOGUS", nil)
	w := httptest.NewRecorder()
	handlePlan(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePlan_ForceAllowedValues(t *testing.T) {
	defer setupMockYahoo(t, 200)()
	for _, f := range []string{"", "AUTO", "SOXL", "SOXS", "soxl"} {
		req := httptest.NewRequest("GET", "/api/plan?capital=10000&risk_pct=1.5&force="+f, nil)
		w := httptest.NewRecorder()
		handlePlan(w, req)
		if w.Code != 200 {
			t.Errorf("force=%q: status=%d body=%s", f, w.Code, w.Body.String())
		}
	}
}

func TestParamOr(t *testing.T) {
	req := httptest.NewRequest("GET", "/?foo=bar", nil)
	if got := paramOr(req, "foo", "x"); got != "bar" {
		t.Errorf("got=%s", got)
	}
	if got := paramOr(req, "missing", "default"); got != "default" {
		t.Errorf("default not used: %s", got)
	}
}

func TestWriteErr(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, 418, "teapot")
	if w.Code != 418 {
		t.Errorf("code=%d", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "teapot" {
		t.Errorf("body=%v", body)
	}
}

func TestHandleDecay_OK(t *testing.T) {
	defer setupMockYahoo(t, 200)()
	req := httptest.NewRequest("GET", "/api/decay?period=1y", nil)
	w := httptest.NewRecorder()
	handleDecay(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
