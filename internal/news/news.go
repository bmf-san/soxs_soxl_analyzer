package news

// Package news fetches headlines from Yahoo Finance search and scores sentiment.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var positiveWords = []string{
	"surge", "soar", "rally", "beat", "beats", "upgrade", "upgraded",
	"record", "growth", "strong", "boost", "rise", "rises", "gain",
	"gains", "bullish", "outperform", "raised", "高騰", "上昇", "好調",
	"急騰", "増益", "強気",
}

var negativeWords = []string{
	"plunge", "drop", "fall", "falls", "miss", "missed", "downgrade",
	"downgraded", "loss", "losses", "decline", "weak", "cut", "cuts",
	"bearish", "underperform", "warning", "concern", "急落", "下落",
	"減益", "弱気", "懸念", "警戒",
}

// Item is one normalized news headline.
type Item struct {
	Time      time.Time `json:"time"`
	Ticker    string    `json:"ticker"`
	Title     string    `json:"title"`
	Publisher string    `json:"publisher"`
	Link      string    `json:"link"`
	Score     int       `json:"score"`
	Sentiment string    `json:"sentiment"`
}

// Summary aggregates news sentiment.
type Summary struct {
	Items    []Item  `json:"items"`
	AvgScore float64 `json:"avg_score"`
	Label    string  `json:"label"`
	Count    int     `json:"count"`
}

type yahooSearchResponse struct {
	News []struct {
		Title               string `json:"title"`
		Publisher           string `json:"publisher"`
		Link                string `json:"link"`
		ProviderPublishTime int64  `json:"providerPublishTime"`
	} `json:"news"`
}

var (
	cacheMu  sync.RWMutex
	cache    = map[string]cachedNews{}
	cacheTTL = 30 * time.Minute
	httpCli  = &http.Client{Timeout: 10 * time.Second}
	// yahooSearchBase is overridable in tests via SetBaseURL.
	yahooSearchBase = "https://query1.finance.yahoo.com"
)

// SetBaseURL overrides the Yahoo search endpoint base. Intended for tests.
func SetBaseURL(u string) { yahooSearchBase = u }

// ClearCache empties the in-memory news cache. Intended for tests.
func ClearCache() {
	cacheMu.Lock()
	cache = map[string]cachedNews{}
	cacheMu.Unlock()
}

type cachedNews struct {
	items   []Item
	expires time.Time
}

func scoreText(text string) int {
	low := strings.ToLower(text)
	score := 0
	for _, w := range positiveWords {
		if strings.Contains(low, strings.ToLower(w)) {
			score++
		}
	}
	for _, w := range negativeWords {
		if strings.Contains(low, strings.ToLower(w)) {
			score--
		}
	}
	return score
}

func sentimentLabel(score int) string {
	switch {
	case score > 0:
		return "🟢 Positive"
	case score < 0:
		return "🔴 Negative"
	}
	return "⚪️ Neutral"
}

// fetchOne hits Yahoo Finance's public search endpoint for a single ticker.
func fetchOne(ticker string) []Item {
	cacheMu.RLock()
	if c, ok := cache[ticker]; ok && time.Now().Before(c.expires) {
		cacheMu.RUnlock()
		return c.items
	}
	cacheMu.RUnlock()

	endpoint := fmt.Sprintf(
		"%s/v1/finance/search?q=%s&newsCount=10&quotesCount=0",
		yahooSearchBase, url.QueryEscape(ticker),
	)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; soxs-analyzer-go/1.0)")
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var data yahooSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	out := make([]Item, 0, len(data.News))
	for _, n := range data.News {
		if n.Title == "" {
			continue
		}
		score := scoreText(n.Title)
		out = append(out, Item{
			Time:      time.Unix(n.ProviderPublishTime, 0).UTC(),
			Ticker:    ticker,
			Title:     n.Title,
			Publisher: n.Publisher,
			Link:      n.Link,
			Score:     score,
			Sentiment: sentimentLabel(score),
		})
	}
	cacheMu.Lock()
	cache[ticker] = cachedNews{items: out, expires: time.Now().Add(cacheTTL)}
	cacheMu.Unlock()
	return out
}

// Fetch retrieves and aggregates news for multiple tickers.
func Fetch(tickers []string, limit int) Summary {
	seen := map[string]struct{}{}
	all := []Item{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tickers {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			items := fetchOne(t)
			mu.Lock()
			defer mu.Unlock()
			for _, it := range items {
				if _, ok := seen[it.Title]; ok {
					continue
				}
				seen[it.Title] = struct{}{}
				all = append(all, it)
			}
		}()
	}
	wg.Wait()

	// sort by time desc
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Time.After(all[i].Time) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	sum := 0
	for _, it := range all {
		sum += it.Score
	}
	avg := 0.0
	if len(all) > 0 {
		avg = float64(sum) / float64(len(all))
	}
	var label string
	switch {
	case avg > 0.3:
		label = "🟢 強気優勢"
	case avg < -0.3:
		label = "🔴 弱気優勢"
	default:
		label = "⚪️ 中立"
	}
	if len(all) == 0 {
		label = "データなし"
	}
	return Summary{Items: all, AvgScore: avg, Label: label, Count: len(all)}
}
