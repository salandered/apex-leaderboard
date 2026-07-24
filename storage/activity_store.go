package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/consumer"
)

type redisActivityStore struct {
	redisLedgerConsumer
}

func newActivityStore(client *redis.Client) *redisActivityStore {
	return &redisActivityStore{redisLedgerConsumer{client: client}}
}

// Own Redis client isolates the blocking ledger read from the request pool.
func NewActivityStore(redisURL string) (consumer.DailyActivityStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("activity store: parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	pingWithRetry(client, opts.Addr)
	return newActivityStore(client), nil
}

func (s *redisActivityStore) ApplyDailyCounts(
	ctx context.Context, increments []consumer.DailyIncrement, ttl time.Duration,
) error {
	pipe := s.client.Pipeline()
	for _, increment := range increments {
		key := activityDailyKey(increment.Date)
		pipe.ZIncrBy(ctx, key, float64(increment.Count), increment.PlayerID) // increments if no key
		pipe.Expire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("storage apply daily activity: %w", err)
	}
	return nil
}
