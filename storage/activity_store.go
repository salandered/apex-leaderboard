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

func NewActivityStore(client *redis.Client) *redisActivityStore {
	return &redisActivityStore{redisLedgerConsumer{client: client}}
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
