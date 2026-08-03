// Works with a redis client provided by go-redis.
package apexredis

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

// satisfies [redis.internal.Logging]
type redisLogger struct{}

func (redisLogger) Printf(ctx context.Context, format string, v ...any) {
	slog.DebugContext(ctx, fmt.Sprintf(format, v...))
}

// component labels this client's commands in the debug log (e.g. "storage", "consumer"):
// two clients built from the same Config stay distinguishable.
// The caller owns the returned client and must Close it.
func New(cfg Config, component string) (*redis.Client, error) {
	cfg.resolve()

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("apexredis: parse redis url: %w", err)
	}
	client := redis.NewClient(opts)

	client.AddHook(logHook{component: component}) // before the ping, so the startup probe is logged

	pingWithRetry(client, opts.Addr, component)
	return client, nil
}

// Probes Redis at startup so an unreachable server is reported early
// (not on first request with the default lazy client).
// On timeout it warns and returns, leaving go-redis to connect lazily.
func pingWithRetry(client *redis.Client, addr, component string) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		err := client.Ping(ctx).Err()
		if err == nil {
			slog.Info("redis connected", "addr", addr, "component", component)
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
		"waited", pingTimeout, "component", component, "error", lastErr)
}
