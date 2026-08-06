package consumer

import (
	"context"
	"time"

	"github.com/salandered/apex/ledger"
)

const DailyActivityID = "daily_activity"

const dailyTTL = 30 * 24 * time.Hour

// Ledger tailing plus this view's own writes.
type DailyActivityStore interface {
	ConsumerStore
	ApplyDailyCounts(ctx context.Context, events []ledger.Event, ttl time.Duration) error
}

// Counts score operations per player per UTC day.
// Not idempotent, because ZINCRBY is not: a replayed batch overcounts. Accepted, the view only
// answers "who was busy today".
func NewDailyActivityConsumer(store DailyActivityStore) *Consumer {
	apply := func(ctx context.Context, events []ledger.Event) error {
		return store.ApplyDailyCounts(ctx, events, dailyTTL)
	}
	return &Consumer{
		store:         store,
		Apply:         apply,
		ID:            DailyActivityID,
		BlockDuration: DefaultBlockDuration,
	}
}
