package news

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockYahooSearch builds a Yahoo-search-compatible response for given headlines.
func mockYahooSearch(headlines []struct {
	Title     string
	Publisher string
}) string {
	news := []map[string]any{}
	for _, h := range headlines {
		news = append(news, map[string]any{
			"title":               h.Title,
			"publisher":           h.Publisher,
			"link":                "https://example.com/" + h.Title,
			"providerPublishTime": time.Now().Unix(),
		})
	}
	b, _ := json.Marshal(map[string]any{"news": news})
	return string(b)
}

func setupMockSearch(t *testing.T, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/finance/search") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	SetBaseURL(srv.URL)
	ClearCache()
	t.Cleanup(func() {
		srv.Close()
		SetBaseURL("https://query1.finance.yahoo.com")
		ClearCache()
	})
}

func TestFetch_AggregatesMultipleTickers(t *testing.T) {
	body := mockYahooSearch([]struct {
		Title     string
		Publisher string
	}{
		{"NVIDIA shares surge on strong earnings beat", "Reuters"},
		{"SOXL plunge on chip downgrade", "Bloomberg"},
		{"Mixed market closes flat", "WSJ"},
	})
	setupMockSearch(t, body, 200)

	sum := Fetch([]string{"SOXL", "SOXS"}, 20)
	if sum.Count == 0 {
		t.Fatal("expected at least one item")
	}
	// dedupe: same titles across both tickers should collapse
	if sum.Count > 3 {
		t.Errorf("expected dedupe, got %d items", sum.Count)
	}
	// labels populated
	if sum.Label == "" {
		t.Error("missing label")
	}
}

func TestFetch_NoItems_LabelIsDataNashi(t *testing.T) {
	setupMockSearch(t, `{"news":[]}`, 200)
	sum := Fetch([]string{"SOXL"}, 10)
	if sum.Count != 0 {
		t.Errorf("expected 0 items, got %d", sum.Count)
	}
	if sum.Label != "データなし" {
		t.Errorf("expected データなし, got %q", sum.Label)
	}
}

func TestFetch_HTTPError_ReturnsEmpty(t *testing.T) {
	setupMockSearch(t, `internal error`, 500)
	sum := Fetch([]string{"SOXL"}, 10)
	if sum.Count != 0 {
		t.Errorf("expected 0 items on HTTP 500, got %d", sum.Count)
	}
}

func TestFetch_MalformedJSON_ReturnsEmpty(t *testing.T) {
	setupMockSearch(t, `not json{{{`, 200)
	sum := Fetch([]string{"SOXL"}, 10)
	if sum.Count != 0 {
		t.Errorf("expected 0 items on bad JSON, got %d", sum.Count)
	}
}

func TestFetch_BullishLabel(t *testing.T) {
	body := mockYahooSearch([]struct {
		Title     string
		Publisher string
	}{
		{"Strong earnings beat boosts gains", "X"},
		{"Surge upgrade rally outperform", "Y"},
		{"Record growth bullish raised", "Z"},
	})
	setupMockSearch(t, body, 200)
	sum := Fetch([]string{"SOXL"}, 10)
	if sum.AvgScore <= 0 {
		t.Errorf("expected positive avg score, got %v", sum.AvgScore)
	}
	if !strings.Contains(sum.Label, "強気") {
		t.Errorf("expected bullish label, got %q", sum.Label)
	}
}

func TestFetch_BearishLabel(t *testing.T) {
	body := mockYahooSearch([]struct {
		Title     string
		Publisher string
	}{
		{"Plunge downgrade loss decline weak", "X"},
		{"Cuts bearish underperform warning", "Y"},
		{"Miss missed concern", "Z"},
	})
	setupMockSearch(t, body, 200)
	sum := Fetch([]string{"SOXL"}, 10)
	if sum.AvgScore >= 0 {
		t.Errorf("expected negative avg score, got %v", sum.AvgScore)
	}
	if !strings.Contains(sum.Label, "弱気") {
		t.Errorf("expected bearish label, got %q", sum.Label)
	}
}

func TestFetch_LimitTrims(t *testing.T) {
	headlines := []struct {
		Title     string
		Publisher string
	}{}
	for i := 0; i < 8; i++ {
		headlines = append(headlines, struct {
			Title     string
			Publisher string
		}{Title: fmt.Sprintf("Headline %d", i), Publisher: "P"})
	}
	body := mockYahooSearch(headlines)
	setupMockSearch(t, body, 200)
	sum := Fetch([]string{"SOXL"}, 3)
	if sum.Count != 3 {
		t.Errorf("expected limit=3, got %d", sum.Count)
	}
}

func TestFetch_CacheHit(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = fmt.Fprint(w, `{"news":[{"title":"hi","publisher":"P","link":"l","providerPublishTime":0}]}`)
	}))
	defer srv.Close()
	SetBaseURL(srv.URL)
	ClearCache()
	t.Cleanup(func() {
		SetBaseURL("https://query1.finance.yahoo.com")
		ClearCache()
	})

	Fetch([]string{"SOXL"}, 10)
	first := hits
	Fetch([]string{"SOXL"}, 10) // second call should hit cache
	if hits != first {
		t.Errorf("expected cache hit on 2nd call (hits=%d, first=%d)", hits, first)
	}
}
