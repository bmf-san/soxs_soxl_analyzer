package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// minimal valid Yahoo chart response
func mockBody(ts []int64, closes []float64, withAdj bool) string {
	tsStr := make([]string, len(ts))
	for i, v := range ts {
		tsStr[i] = fmt.Sprintf("%d", v)
	}
	closesStr := make([]string, len(closes))
	volsStr := make([]string, len(closes))
	for i, v := range closes {
		closesStr[i] = fmt.Sprintf("%v", v)
		volsStr[i] = "1000"
	}
	adjBlock := ""
	if withAdj {
		adjBlock = fmt.Sprintf(`,"adjclose":[{"adjclose":[%s]}]`, strings.Join(closesStr, ","))
	}
	return fmt.Sprintf(`{
      "chart": {
        "result": [{
          "timestamp": [%s],
          "indicators": {
            "quote": [{
              "open":   [%s],
              "high":   [%s],
              "low":    [%s],
              "close":  [%s],
              "volume": [%s]
            }]%s
          }
        }],
        "error": null
      }
    }`, strings.Join(tsStr, ","),
		strings.Join(closesStr, ","), strings.Join(closesStr, ","),
		strings.Join(closesStr, ","), strings.Join(closesStr, ","),
		strings.Join(volsStr, ","), adjBlock)
}

func TestParseChartResponse_Valid(t *testing.T) {
	body := mockBody([]int64{1700000000, 1700086400, 1700172800}, []float64{100, 101, 102}, false)
	s, err := parseChartResponse(strings.NewReader(body), "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Bars) != 3 {
		t.Fatalf("bars=%d", len(s.Bars))
	}
	if s.Symbol != "TEST" {
		t.Errorf("symbol=%v", s.Symbol)
	}
	if s.Bars[2].Close != 102 {
		t.Errorf("close[2]=%v", s.Bars[2].Close)
	}
}

func TestParseChartResponse_PrefersAdjClose(t *testing.T) {
	body := `{
      "chart": {
        "result": [{
          "timestamp": [1700000000, 1700086400],
          "indicators": {
            "quote": [{
              "open":[10,11],"high":[12,13],"low":[9,10],
              "close":[11,12],"volume":[1000,2000]
            }],
            "adjclose":[{"adjclose":[10.5, 11.5]}]
          }
        }],
        "error": null
      }
    }`
	s, err := parseChartResponse(strings.NewReader(body), "T")
	if err != nil {
		t.Fatal(err)
	}
	// adjusted close should override raw close
	if s.Bars[0].Close != 10.5 || s.Bars[1].Close != 11.5 {
		t.Fatalf("adj close not used: %+v", s.Bars)
	}
}

func TestParseChartResponse_YahooError(t *testing.T) {
	body := `{"chart":{"result":null,"error":{"code":"Not Found","description":"No data found"}}}`
	_, err := parseChartResponse(strings.NewReader(body), "X")
	if err == nil || !strings.Contains(err.Error(), "yahoo error") {
		t.Fatalf("expected yahoo error, got %v", err)
	}
}

func TestParseChartResponse_EmptyResult(t *testing.T) {
	body := `{"chart":{"result":[],"error":null}}`
	_, err := parseChartResponse(strings.NewReader(body), "X")
	if err == nil {
		t.Fatal("expected error for empty result")
	}
}

func TestParseChartResponse_MalformedJSON(t *testing.T) {
	_, err := parseChartResponse(strings.NewReader("{not json"), "X")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestParseChartResponse_NilClosesSkipped(t *testing.T) {
	body := `{
      "chart": {
        "result": [{
          "timestamp": [1700000000, 1700086400, 1700172800],
          "indicators": {
            "quote": [{
              "open":[10,null,12],"high":[11,null,13],"low":[9,null,11],
              "close":[10, null, 12], "volume":[1000,null,3000]
            }]
          }
        }],
        "error": null
      }
    }`
	s, err := parseChartResponse(strings.NewReader(body), "T")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Bars) != 2 {
		t.Fatalf("expected 2 bars (null skipped), got %d", len(s.Bars))
	}
}

func TestParseChartResponse_AllNilFails(t *testing.T) {
	body := `{
      "chart": {
        "result": [{
          "timestamp": [1, 2],
          "indicators": {
            "quote": [{"open":[null,null],"high":[null,null],"low":[null,null],"close":[null,null],"volume":[null,null]}]
          }
        }],
        "error": null
      }
    }`
	_, err := parseChartResponse(strings.NewReader(body), "T")
	if err == nil {
		t.Fatal("expected error when all bars unusable")
	}
}

func TestLoadSeries_MockServer(t *testing.T) {
	ClearCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v8/finance/chart/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mockBody([]int64{1, 2, 3, 4, 5}, []float64{100, 101, 102, 101, 103}, true))
	}))
	defer srv.Close()
	orig := yahooBaseURL
	SetBaseURL(srv.URL)
	defer SetBaseURL(orig)

	s, err := LoadSeries("FOO", "1mo")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Bars) != 5 {
		t.Fatalf("bars=%d", len(s.Bars))
	}

	// second call should hit cache (mock server would still respond but cache should work)
	s2, _ := LoadSeries("FOO", "1mo")
	if &s.Bars[0] != &s2.Bars[0] {
		t.Error("expected cached series pointer to be reused")
	}
}

func TestLoadSeries_HTTPError(t *testing.T) {
	ClearCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	orig := yahooBaseURL
	SetBaseURL(srv.URL)
	defer SetBaseURL(orig)

	_, err := LoadSeries("BAR", "1mo")
	if err == nil {
		t.Fatal("expected http error")
	}
}

func TestLoadMany_PartialFailure(t *testing.T) {
	ClearCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "BAD") {
			http.Error(w, "fail", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mockBody([]int64{1, 2}, []float64{50, 51}, false))
	}))
	defer srv.Close()
	orig := yahooBaseURL
	SetBaseURL(srv.URL)
	defer SetBaseURL(orig)

	out := LoadMany([]string{"OK1", "BAD", "OK2"}, "1mo")
	if _, ok := out["OK1"]; !ok {
		t.Error("OK1 missing")
	}
	if _, ok := out["OK2"]; !ok {
		t.Error("OK2 missing")
	}
	if _, ok := out["BAD"]; ok {
		t.Error("BAD should be excluded on failure")
	}
}
