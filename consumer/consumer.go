package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/salandered/apex/ledger"
)

// How long an XREAD parks if no new events in the stream.
const DefaultBlockDuration = 5 * time.Second

const (
	cursorHead   = "0-0" // fold from stream head: full catch-up on first boot
	batchCount   = 10
	retryBackoff = time.Second
)

// ConsumerStore is what Consumer needs.
// (interface "discovered" on a client side, intentionally not a part of the [storage] package)
type ConsumerStore interface {
	LoadCursor(ctx context.Context, consumer string) (cursor string, found bool, err error)
	SaveCursor(ctx context.Context, consumer, cursor string) error
	ReadLedgerBatch(ctx context.Context, after string, limit int64, block time.Duration) (LedgerBatch, error)
}

// ApplyFunc writes one batch into a view derived from the ledger.
type ApplyFunc func(ctx context.Context, events []ledger.Event) error

type LedgerBatch struct {
	Events   []ledger.Event
	Rejected []RejectedEntry
	LastID   string
}

type RejectedEntry struct {
	ID  string
	Err error
}

// Consumer tails the ledger and feeds one view.
type Consumer struct {
	// cursor name derives from it. Changing it re-folds the ledger from the head (!)
	ID    string
	store ConsumerStore
	Apply ApplyFunc
	// How long consumer blocks on a stream read.
	// Callers waiting for a clean stop should allow this long (+ some margin).
	BlockDuration time.Duration
}

func new(store ConsumerStore, id string, apply ApplyFunc) *Consumer {
	return &Consumer{
		store:         store,
		Apply:         apply,
		ID:            id,
		BlockDuration: DefaultBlockDuration, // may be config in the future
	}
}

// Run tails the ledger until ctx is cancelled (gracefull shutdown).
// Batch failures are logged and retried.
func (c *Consumer) Run(ctx context.Context) error {
	id := c.ID
	slog.InfoContext(ctx, "consumer: started", "consumer", id)
	defer slog.InfoContext(ctx, "consumer: stopped", "consumer", id)

	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err := c.processOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.ErrorContext(ctx, "consumer: batch failed, retrying",
				"consumer", id, "error", err)
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
func (c *Consumer) processOnce(ctx context.Context) (int, error) {
	id := c.ID
	cursor, found, err := c.store.LoadCursor(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("consumer %s: load cursor: %w", id, err)
	}
	if !found {
		cursor = cursorHead
	}

	batch, err := c.store.ReadLedgerBatch(ctx, cursor, batchCount, c.BlockDuration)
	if err != nil {
		return 0, fmt.Errorf("consumer %s: read ledger: %w", id, err)
	}
	n := len(batch.Events) + len(batch.Rejected)
	if n == 0 {
		return 0, nil
	}
	slog.DebugContext(ctx, "consumer: ledger batch read",
		"consumer", id,
		"events", len(batch.Events),
		"rejected", len(batch.Rejected),
		"last_id", batch.LastID,
	)

	for _, rejected := range batch.Rejected {
		slog.WarnContext(ctx, "consumer: skipping malformed ledger entry",
			"consumer", id, "id", rejected.ID, "error", rejected.Err)
	}

	if len(batch.Events) > 0 {
		if err := c.Apply(ctx, batch.Events); err != nil {
			return 0, fmt.Errorf("consumer %s: apply batch: %w", id, err)
		}
	}

	if err := c.store.SaveCursor(ctx, id, batch.LastID); err != nil {
		return n, fmt.Errorf("consumer %s: persist cursor: %w", id, err)
	}
	return n, nil
}
