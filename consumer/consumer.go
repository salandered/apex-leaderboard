package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/salandered/apex/ledger"
)

const (
	dailyActivityConsumerName = "daily_activity"
	cursorHead                = "0-0" // fold from stream head: full catch-up on first boot
	batchCount                = 10
	blockDuration             = 5 * time.Second
	dailyTTL                  = 30 * 24 * time.Hour
	retryBackoff              = time.Second
)

type ConsumerStore interface {
	LoadCursor(ctx context.Context, consumer string) (cursor string, found bool, err error)
	SaveCursor(ctx context.Context, consumer, cursor string) error
	ReadLedgerBatch(ctx context.Context, after string, limit int64, block time.Duration) (LedgerBatch, error)
}

type DailyActivityStore interface {
	ConsumerStore
	ApplyDailyCounts(ctx context.Context, increments []DailyIncrement, ttl time.Duration) error
}

type LedgerBatch struct {
	Events   []ledger.Event
	Rejected []RejectedEntry
	LastID   string
}

type RejectedEntry struct {
	ID  string
	Err error
}

type DailyIncrement struct {
	Date     string
	PlayerID string
	Count    int64
}

type DailyActivityConsumer struct {
	store DailyActivityStore
	name  string
}

func NewDailyActivityConsumer(store DailyActivityStore) *DailyActivityConsumer {
	return &DailyActivityConsumer{store: store, name: dailyActivityConsumerName}
}

// Run tails the ledger until ctx is cancelled. Batch failures are logged and retried.
// Later should be gracefully stopped via ctx (graceful shutdown is not implemented)
func (c *DailyActivityConsumer) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err := c.processOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.ErrorContext(ctx, "activity consumer: batch failed, retrying", "error", err)
			select {
			case <-time.After(retryBackoff):
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// At-least-once: entries are applied before the cursor is persisted, so a crash
// between the two re-applies the batch on restart.
// Returns the number of processed events (including rejected)
func (c *DailyActivityConsumer) processOnce(ctx context.Context) (int, error) {
	cursor, found, err := c.store.LoadCursor(ctx, c.name)
	if err != nil {
		return 0, fmt.Errorf("activity consumer: load cursor: %w", err)
	}
	if !found {
		cursor = cursorHead
	}

	batch, err := c.store.ReadLedgerBatch(ctx, cursor, batchCount, blockDuration)
	if err != nil {
		return 0, fmt.Errorf("activity consumer: read ledger: %w", err)
	}
	n := len(batch.Events) + len(batch.Rejected)
	if n == 0 {
		return 0, nil
	}
	slog.DebugContext(ctx, "activity consumer: ledger batch read",
		"events", len(batch.Events),
		"rejected", len(batch.Rejected),
		"last_id", batch.LastID,
	)

	for _, rejected := range batch.Rejected {
		slog.WarnContext(ctx, "activity consumer: skipping malformed ledger entry",
			"id", rejected.ID, "error", rejected.Err)
	}

	increments := make([]DailyIncrement, 0, len(batch.Events))
	for _, event := range batch.Events {
		increments = append(increments, DailyIncrement{
			Date:     event.CreatedAt.Format(time.DateOnly),
			PlayerID: event.PlayerID,
			Count:    1,
		})
	}
	if len(increments) > 0 {
		if err := c.store.ApplyDailyCounts(ctx, increments, dailyTTL); err != nil {
			return 0, fmt.Errorf("activity consumer: apply batch: %w", err)
		}
	}

	if err := c.store.SaveCursor(ctx, c.name, batch.LastID); err != nil {
		return n, fmt.Errorf("activity consumer: persist cursor: %w", err)
	}
	return n, nil
}
