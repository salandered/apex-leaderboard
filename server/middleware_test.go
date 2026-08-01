package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoveryMiddlewarePanicBeforeWriteReturns500(t *testing.T) {
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	rec := httptest.NewRecorder()

	// when
	loggingMiddleware(recoveryMiddleware(h)).ServeHTTP(rec, genericRequest())

	// then
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

func TestRecoveryMiddlewarePanicAfterPartialWriteKeepsOriginalStatus(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"partial":`)
		panic("boom")
	})
	rec := httptest.NewRecorder()

	// when
	loggingMiddleware(recoveryMiddleware(h)).ServeHTTP(rec, genericRequest())

	// then
	// headers already flushed -> recovery must NOT rewrite the status to 500
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if rec.Body.String() != `{"partial":` {
		t.Fatalf("partial body not preserved: %q", rec.Body.String())
	}
}

func TestRecoveryMiddlewareOnErrAbortHandlerRepanics(t *testing.T) {
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})
	rec := httptest.NewRecorder()

	// then
	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint
			t.Fatalf("want ErrAbortHandler re-panicked, got %v", v)
		}
	}()

	// when
	recoveryMiddleware(h).ServeHTTP(rec, genericRequest())

	// then
	t.Fatal("expected re-panic, did not happen")
}

func genericRequest() *http.Request {
	return httptest.NewRequest("GET", "/x", nil)
}
