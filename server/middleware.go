package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/salandered/apex/requestid"
)

// A wrapper that embeds the [http.ResponseWriter] and some of its methods.
// Captures the response status and size for logging.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK) // mirror net/http: first Write commits a 200
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Creates server-generated correlation id.
// Adds it to the X-Request-ID response header.
// Adds it in the request context.
// The request header is intentionally ignored (see docs).
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := requestid.New()
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, req.WithContext(requestid.WithID(req.Context(), id)))
	})
}

// recoveryMiddleware catches a panic from the downstream handler and turnes it into a logged 500.
// Should be inside loggingMiddleware so a new 500 would be logged.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler { //nolint:errorlint // panic value, not a wrapped err
				panic(v) // sentinel, we repanic
			}
			slog.LogAttrs(req.Context(), slog.LevelError, "panic recovered",
				slog.String("request_id", requestid.FromContext(req.Context())),
				slog.Any("panic", v),
				slog.String("stack", string(debug.Stack())),
			)
			// don't write header if the handler already started doing it
			rec, ok := w.(*statusRecorder)
			if !ok || !rec.wroteHeader { // !ok should not happen
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, req)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK} // default 200

		next.ServeHTTP(rec, req)

		level := slog.LevelInfo
		switch {
		case rec.status >= 500:
			level = slog.LevelError
		case rec.status >= 400:
			level = slog.LevelWarn
		}
		slog.LogAttrs(
			req.Context(),
			level,
			"request",
			slog.String("request_id", requestid.FromContext(req.Context())),
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
			slog.Int("bytes", rec.bytes),
		)
	})
}
