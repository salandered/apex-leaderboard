package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterAllowsBurstThenRejects(t *testing.T) {
	limiter := testRateLimiter(t, 1, 3)
	handler := limiter.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	var codes []int
	for range 5 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, requestFrom("10.1.2.3:5000"))
		codes = append(codes, rec.Code)
	}

	want := []int{200, 200, 200, 429, 429} // burst of 3, then the bucket is empty
	for i, code := range codes {
		if code != want[i] {
			t.Fatalf("request %d: want %d, got %d (all: %v)", i, want[i], code, codes)
		}
	}
}

func TestRateLimiterRejectionCarriesRetryAfterAndJSONBody(t *testing.T) {
	limiter := testRateLimiter(t, 1, 1)
	handler := limiter.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), requestFrom("10.1.2.3:5000")) // drains the bucket
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestFrom("10.1.2.3:5000"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After: want 1, got %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", got)
	}
	if got := rec.Body.String(); got != `{"error":"rate limit exceeded"}` {
		t.Errorf("body: got %q", got)
	}
}

func TestRateLimiterKeepsSeparateBucketsPerAddress(t *testing.T) {
	limiter := testRateLimiter(t, 1, 1)
	handler := limiter.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), requestFrom("10.1.2.3:5000"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestFrom("10.1.2.4:5000")) // other client, own bucket

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for a different address, got %d", rec.Code)
	}
}

func TestRateLimiterSkipsHealthEndpoints(t *testing.T) {
	limiter := testRateLimiter(t, 1, 1)
	handler := limiter.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	for _, path := range []string{"/livez", "/readyz"} {
		for range 5 {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "10.1.2.3:5000"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: want 200, got %d", path, rec.Code)
			}
		}
	}
}

func TestClientKey(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
		name       string
	}{
		{"10.1.2.3:5000", "10.1.2.3", "ipv4 with port"},
		{"10.1.2.3", "10.1.2.3", "ipv4 without port"},
		{"[2001:db8::1]:5000", "2001:db8::1", "ipv6 with port"},
		{"garbage", "garbage", "unparsable, used as is"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientKey(tt.remoteAddr); got != tt.want {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestRateLimiterIgnoresForwardedForHeader(t *testing.T) {
	limiter := testRateLimiter(t, 1, 1)
	handler := limiter.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), requestFrom("10.1.2.3:5000")) // drains the bucket
	rec := httptest.NewRecorder()
	// a client claiming another address must not get a fresh bucket
	handler.ServeHTTP(rec, requestFrom("10.1.2.3:5000", "9.9.9.9"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rec.Code)
	}
}

func testRateLimiter(t *testing.T, rps float64, burst int) *rateLimiter {
	t.Helper()
	cfg := serverConfig{}
	if err := WithRateLimit(rps, burst)(&cfg); err != nil {
		t.Fatalf("WithRateLimit: %v", err)
	}
	return newRateLimiter(cfg.rateLimit)
}

func requestFrom(remoteAddr string, forwardedFor ...string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/boards", nil)
	req.RemoteAddr = remoteAddr
	for _, v := range forwardedFor {
		req.Header.Add("X-Forwarded-For", v)
	}
	return req
}
