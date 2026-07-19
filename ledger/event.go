package ledger

import "time"

type EventType string

const (
	EventSet       EventType = "set"       // Amount is the absolute value
	EventIncrement EventType = "increment" // Amount is the delta (may be negative)
)

// Uses auto-generated Redis stream entry id.
// formerly known as ScoreEvent
type Event struct {
	ID        string    // uses auto-generated Redis ID
	Type      EventType //
	PlayerID  string    //
	BoardID   string    //
	Amount    float64   //
	RequestID string    // client-supplied idempotency key
	CreatedAt time.Time // derived from the ID
}
