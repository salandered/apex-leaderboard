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
	ApplyDailyCounts(ctx context.Context, increments []DailyIncrement, ttl time.Duration) error
}

type DailyIncrement struct {
	Date     string
	PlayerID string
	Count    int64
}

// Counts score operations per player per UTC day.
// Not idempotent, because ZINCRBY is not: a replayed batch overcounts. Accepted, the view only
// answers "who was busy today".
func NewDailyActivityConsumer(store DailyActivityStore) *Consumer {
	apply := func(ctx context.Context, events []ledger.Event) error {
		return store.ApplyDailyCounts(ctx, dailyIncrements(events), dailyTTL)
	}
	return new(store, DailyActivityID, apply)
}

// One tick per event, bucketed by the event's UTC day.
func dailyIncrements(events []ledger.Event) []DailyIncrement {
	increments := make([]DailyIncrement, 0, len(events))
	for _, event := range events {
		increments = append(increments, DailyIncrement{
			Date:     event.CreatedAt.Format(time.DateOnly),
			PlayerID: event.PlayerID,
			Count:    1,
		})
	}
	return increments
}
