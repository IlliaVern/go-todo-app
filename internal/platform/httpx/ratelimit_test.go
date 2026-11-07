package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter(3)
	l.now = func() time.Time { return clock }
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	do := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/sign-in", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := range 3 {
		if rec := do("192.0.2.1:1000"); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status %d, want burst of 3 allowed", i+1, rec.Code)
		}
	}
	rec := do("192.0.2.1:2000") // same IP, different port
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "20" {
		t.Fatalf("4th request: status %d Retry-After %q, want 429 and 20", rec.Code, rec.Header().Get("Retry-After"))
	}

	if rec := do("192.0.2.2:1000"); rec.Code != http.StatusNoContent {
		t.Errorf("other client: status %d, want its own bucket", rec.Code)
	}

	clock = clock.Add(20 * time.Second) // one token refills every 60s/3
	if rec := do("192.0.2.1:1000"); rec.Code != http.StatusNoContent {
		t.Errorf("after refill: status %d, want allowed", rec.Code)
	}
}

func TestRateLimiterGroupsIPv6By64(t *testing.T) {
	l := NewRateLimiter(1)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	do := func(remoteAddr string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do("[2001:db8:1:2::1]:443"); code != http.StatusNoContent {
		t.Fatalf("first: %d", code)
	}
	if code := do("[2001:db8:1:2::ffff]:443"); code != http.StatusTooManyRequests {
		t.Errorf("same /64: %d, want 429", code)
	}
	if code := do("[2001:db8:1:3::1]:443"); code != http.StatusNoContent {
		t.Errorf("other /64: %d, want allowed", code)
	}
}

func TestRateLimiterForgetsIdleClients(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter(1)
	l.now = func() time.Time { return clock }
	l.allow(clientKey(&http.Request{RemoteAddr: "192.0.2.1:1"}))

	clock = clock.Add(idleTTL + time.Minute)
	l.allow(clientKey(&http.Request{RemoteAddr: "192.0.2.2:1"}))
	if _, ok := l.clients[clientKey(&http.Request{RemoteAddr: "192.0.2.1:1"})]; ok || len(l.clients) != 1 {
		t.Errorf("idle client kept: %d buckets", len(l.clients))
	}
}
