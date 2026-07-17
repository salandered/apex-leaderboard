package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	pingTimeout       = 5 * time.Second
	pingRetryInterval = 250 * time.Millisecond
)

// go-redis writes its diagnostic logs (e.g. pool dial failures) to stderr via a package-global logger.
// We route them to slog via redisLogger.
func init() {
	redis.SetLogger(redisLogger{})
}

type redisLogger struct{}

func (redisLogger) Printf(ctx context.Context, format string, v ...any) {
	slog.DebugContext(ctx, fmt.Sprintf(format, v...))
}

// Probes Redis at startup so an unreachable server is reported early
// (not first request with default lazy client)
// On timeout it warns and returns, leaving go-redis to connect lazily.
func pingWithRetry(client *redis.Client, addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		err := client.Ping(ctx).Err()
		if err == nil {
			slog.Info("redis connected", "addr", addr)
			return
		}
		if ctx.Err() != nil { // prefer a real dial error over the deadline
			if lastErr == nil {
				lastErr = err
			}
			break
		}
		lastErr = err
		slog.Debug("redis not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-time.After(pingRetryInterval):
		case <-ctx.Done():
		}
	}
	slog.Warn("redis unreachable at startup, continuing (will connect on first use)",
		"waited", pingTimeout, "error", lastErr)
}

func NewStorage(url string) (Storage, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("storage: parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	pingWithRetry(client, opts.Addr)
	return &redisStorage{client: client}, nil
}
