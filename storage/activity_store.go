package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/ledger"
)

type redisActivityStore struct {
	client *redis.Client
}

func newActivityStore(client *redis.Client) *redisActivityStore {
	return &redisActivityStore{client: client}
}

// TODO: cursor methods and ReadLedgerBatch should be separated as generic methods (like cursor repo)

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

func (s *redisActivityStore) LoadCursor(
	ctx context.Context, consumerName string,
) (string, bool, error) {
	cursor, err := s.client.Get(ctx, consumerCursorKey(consumerName)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("storage load consumer cursor: %w", err)
	}
	return cursor, true, nil
}

func (s *redisActivityStore) SaveCursor(
	ctx context.Context, consumerName, cursor string,
) error {
	if err := s.client.Set(ctx, consumerCursorKey(consumerName), cursor, 0).Err(); err != nil {
		return fmt.Errorf("storage save consumer cursor: %w", err)
	}
	return nil
}

func (s *redisActivityStore) ReadLedgerBatch(
	ctx context.Context, after string, limit int64, block time.Duration,
) (consumer.LedgerBatch, error) {
	streams, err := s.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{ledgerKey, after},
		Count:   limit,
		Block:   block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return consumer.LedgerBatch{}, nil
	}
	if err != nil {
		return consumer.LedgerBatch{}, fmt.Errorf("storage read ledger batch: %w", err)
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return consumer.LedgerBatch{}, nil
	}

	messages := streams[0].Messages
	batch := consumer.LedgerBatch{
		Events:   make([]ledger.Event, 0, len(messages)),
		Rejected: make([]consumer.RejectedEntry, 0),
		LastID:   messages[len(messages)-1].ID,
	}
	for _, entry := range messages {
		event, err := entryToEvent(entry)
		if err != nil {
			batch.Rejected = append(batch.Rejected, consumer.RejectedEntry{ID: entry.ID, Err: err})
			continue
		}
		batch.Events = append(batch.Events, event)
	}
	return batch, nil
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
