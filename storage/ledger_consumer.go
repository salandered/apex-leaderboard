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

// redisLedgerConsumer is the storage that tracks cursor and
// batch tails the ledger stream.
type redisLedgerConsumer struct {
	client *redis.Client
}

func (s *redisLedgerConsumer) LoadCursor(
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

func (s *redisLedgerConsumer) SaveCursor(
	ctx context.Context, consumerName, cursor string,
) error {
	if err := s.client.Set(ctx, consumerCursorKey(consumerName), cursor, 0).Err(); err != nil {
		return fmt.Errorf("storage save consumer cursor: %w", err)
	}
	return nil
}

// Blocks for 'block' duration (see XREAD BLOCK)
func (s *redisLedgerConsumer) ReadLedgerBatch(
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
