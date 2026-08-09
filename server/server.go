package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultPort            = 8090
	DefaultShutdownTimeout = 10 * time.Second
	DefaultRateLimitBurst  = 20
)

var (
	ErrServer   = errors.New("server")
	ErrServe    = fmt.Errorf("%w: serve failed", ErrServer)
	ErrOption   = fmt.Errorf("%w: invalid option", ErrServer)
	ErrShutdown = fmt.Errorf("%w: shutdown incomplete", ErrServer)
)

type serverConfig struct {
	port            int
	shutdownTimeout time.Duration
	rateLimit       rateLimitConfig
}

type Option func(cfg *serverConfig) error

func WithPort(port int) Option {
	return func(cfg *serverConfig) error {
		if port <= 0 || port > 65535 {
			return fmt.Errorf("%w: port should be in 1..65535", ErrOption)
		}
		cfg.port = port
		return nil
	}
}

func WithShutdownTimeout(st time.Duration) Option {
	return func(cfg *serverConfig) error {
		if st <= 0 {
			return fmt.Errorf("%w: shutdown timeout should be positive", ErrOption)
		}
		cfg.shutdownTimeout = st
		return nil
	}
}

func WithRateLimit(rps float64, burst int) Option {
	return func(cfg *serverConfig) error {
		if rps <= 0 {
			return fmt.Errorf("%w: rate limit rps should be positive", ErrOption)
		}
		if burst < 1 {
			return fmt.Errorf("%w: rate limit burst should be at least 1", ErrOption)
		}
		cfg.rateLimit.rps = rps
		cfg.rateLimit.burst = burst
		return nil
	}
}

// Start runs the server until ctx is cancelled, then shuts down in-flight requests.
// The caller owns error logging.
func Start(ctx context.Context, handler http.Handler, opts ...Option) error {
	cfg := serverConfig{port: DefaultPort, shutdownTimeout: DefaultShutdownTimeout}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return err
		}
	}

	// Add rate limiter middleware first,
	// a rejected request is still logged and carries a request id.
	if cfg.rateLimit.enabled() {
		limiter := newRateLimiter(cfg.rateLimit)
		go limiter.runCleanup(ctx)
		handler = limiter.middleware(handler)
		slog.Info("rate limiting enabled", "rps", cfg.rateLimit.rps, "burst", cfg.rateLimit.burst)
	}

	srv := &http.Server{
		Addr:           ":" + strconv.Itoa(cfg.port),
		Handler:        requestIDMiddleware(loggingMiddleware(recoveryMiddleware(handler))),
		IdleTimeout:    time.Minute,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
		// net/http writes its own diagnostics here (superfluous WriteHeader, bad
		// Content-Length, TLS handshake errors). This should be caught by our slog.
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}
	slog.Info("starting server", "addr", srv.Addr)

	// Start the server. Unexpected error is sent to errServeCh
	errServeCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errServeCh <- err
		}
	}()

	// Block until the ctx is cancelled, or the server sent an unexpected error
	select {
	case err := <-errServeCh:
		return fmt.Errorf("%w: %w", ErrServe, err) // ListenAndServe returned unexpected error -> fail fast
	case <-ctx.Done(): // signal -> graceful shutdown
	}

	// Shutdown.
	slog.Info("shutting down server", "timeout", cfg.shutdownTimeout)
	shutDownStart := time.Now()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil { // when succeeds -> ListenAndServe returns ErrServerClosed
		return fmt.Errorf("%w: %w", ErrShutdown, err)
	}

	slog.Info("server stopped", "shutdown took", time.Since(shutDownStart))
	return nil
}
