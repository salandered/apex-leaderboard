package consumer

import (
	"context"

	"github.com/salandered/apex/ledger"
)

const PlayerHistoryID = "player_history"

// Ledger tailing plus this view's own writes.
type PlayerHistoryStore interface {
	ConsumerStore
	ApplyPlayerHistory(ctx context.Context, events []ledger.Event) error
}

// Indexes each event id under its (player, board), so history reads a page, not the stream.
// The store writes pointers keyed by entry id, so a replayed batch is a no-op.
func NewPlayerHistoryConsumer(store PlayerHistoryStore) *Consumer {
	return &Consumer{
		store:         store,
		Apply:         store.ApplyPlayerHistory,
		ID:            PlayerHistoryID,
		BlockDuration: DefaultBlockDuration,
	}
}
