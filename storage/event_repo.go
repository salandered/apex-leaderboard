package storage

import (
	"context"
	"fmt"

	"github.com/salandered/apex/ledger"
)

// Events after the exclusive cursor, oldest first.
func (rs *redisStorage) ListEventsAfter(
	ctx context.Context, after string, limit int64,
) ([]ledger.Event, error) {
	if limit <= 0 {
		return []ledger.Event{}, nil
	}
	// XRangeN acts as XRANGE COUNT (https://redis.io/docs/latest/commands/xrange/#optional-arguments)
	entries, err := rs.client.XRangeN(ctx, ledgerKey, "("+after, "+", limit).Result()
	if err != nil {
		return nil, fmt.Errorf("storage list ledger events: %w", err)
	}

	events := make([]ledger.Event, 0, len(entries))
	for _, entry := range entries {
		event, err := entryToEvent(entry)
		if err != nil {
			return nil, fmt.Errorf("storage list ledger events: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}
