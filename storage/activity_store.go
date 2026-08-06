package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/ledger"
)

type redisActivityStore struct {
	redisLedgerConsumer
}

func NewActivityStore(client *redis.Client) *redisActivityStore {
	return &redisActivityStore{redisLedgerConsumer{client: client}}
}

// One tick per event, bucketed by the event's UTC day.
func (s *redisActivityStore) ApplyDailyCounts(
	ctx context.Context, events []ledger.Event, ttl time.Duration,
) error {
	pipe := s.client.Pipeline()
	for _, event := range events {
		key := activityDailyKey(event.CreatedAt.Format(time.DateOnly))
		pipe.ZIncrBy(ctx, key, 1, event.PlayerID) // increments if no key
		pipe.Expire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("storage apply daily activity: %w", err)
	}
	return nil
}
