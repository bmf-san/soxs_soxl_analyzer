package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimit_AllowsBurstThenBlocks(t *testing.T) {
	// reset buckets
	rlMu.Lock()
	rlBuckets = map[string]*bucket{}
	rlMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(withMiddleware(mux))
	defer srv.Close()

	allowed := 0
	limited := 0
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/api/ping", nil)
		req.Header.Set("X-Forwarded-For", "9.9.9.9")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			allowed++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	if allowed < 1 || allowed > 15 {
		t.Errorf("burst allowed unusual count: %d", allowed)
	}
	if limited == 0 {
		t.Errorf("expected some requests to be rate-limited (allowed=%d)", allowed)
	}
}

func TestRateLimit_DifferentIPsIndependent(t *testing.T) {
	rlMu.Lock()
	rlBuckets = map[string]*bucket{}
	rlMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(withMiddleware(mux))
	defer srv.Close()

	// IP A: exhaust budget
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/api/ping", nil)
		req.Header.Set("X-Forwarded-For", "1.1.1.1")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
	// IP B: should still pass first request
	req, _ := http.NewRequest("GET", srv.URL+"/api/ping", nil)
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("different IP should not be limited by another IP's quota; got %d", resp.StatusCode)
	}
}

func TestRateLimit_StaticPassesThrough(t *testing.T) {
	rlMu.Lock()
	rlBuckets = map[string]*bucket{}
	rlMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(withMiddleware(mux))
	defer srv.Close()

	// 100 requests to / (not /api/) — none should be rate limited
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		req.Header.Set("X-Forwarded-For", "3.3.3.3")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("non-api path should never be rate limited (req %d)", i)
		}
	}
}
