package server

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/salandered/apex/handlers"
)

const (
	clientTTL       = 3 * time.Minute // bucket entries idle for longer are dropped
	cleanupInterval = time.Minute
	// Should be capped but we don't expect it to be hit anytime soon.
	maxClients = 50_000
)

var (
	errRateLimited    = errors.New("rate limit exceeded")
	errTooManyClients = errors.New("rate limiter table full")
)

type rateLimitConfig struct {
	rps   float64
	burst int
}

func (c rateLimitConfig) enabled() bool { return c.rps > 0 }

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientLimiter

	rps           rate.Limit
	burst         int
	retryAfterSec string // seconds until the next token
}

func newRateLimiter(cfg rateLimitConfig) *rateLimiter {
	return &rateLimiter{
		clients:       make(map[string]*clientLimiter),
		rps:           rate.Limit(cfg.rps),
		burst:         cfg.burst,
		retryAfterSec: strconv.Itoa(max(1, int(math.Ceil(1/cfg.rps)))),
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := rl.allow(req.URL.Path, req.RemoteAddr)
		if err == nil {
			next.ServeHTTP(w, req)
			return
		}

		status := http.StatusTooManyRequests
		if errors.Is(err, errTooManyClients) {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Retry-After", rl.retryAfterSec)
		handlers.WriteErrorToResponse(req.Context(), w, err, status)
	})
}

// Reports the reason to reject the request, nil if it may proceed.
func (rl *rateLimiter) allow(path string, remoteAddr string) error {
	if path == "/livez" || path == "/readyz" {
		return nil
	}

	key := clientKey(remoteAddr)
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	cl, ok := rl.clients[key]

	// Add to buckets if new client
	if !ok {
		if len(rl.clients) >= maxClients {
			return errTooManyClients
		}
		cl = &clientLimiter{
			limiter: rate.NewLimiter(rl.rps, rl.burst),
		}
		rl.clients[key] = cl
	}

	cl.lastSeen = now

	if !cl.limiter.Allow() {
		return errRateLimited
	}
	return nil
}

// Drops entries that are idle for more than [clientTTL]
func (rl *rateLimiter) runCleanup(ctx context.Context) {
	tickCh := time.Tick(cleanupInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tickCh:
			rl.mu.Lock()
			for key, c := range rl.clients {
				if time.Since(c.lastSeen) > clientTTL {
					delete(rl.clients, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// The address the request arrives from.
// X-Forwarded-For and X-Real-IP are ignored: a client might set it freely.
//
// TODO: When behind a proxy, a single bucket is used for all of the traffic
func clientKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr // no port to strip
	}
	return host
}
