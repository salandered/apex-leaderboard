package storage

import "time"

// EventType enumerates the operations that can appear in the ledger.
type EventType string

const (
	EventSet       EventType = "set"       // Amount is the absolute value
	EventIncrement EventType = "increment" // Amount is the delta (may be negative)
)

// stream event fields. These are the field keys stored in each score:events
// entry and read back by History / Rebuild
// Must match the Lua write script.
const (
	eventFieldType      = "type"
	eventFieldPlayerID  = "player_id"
	eventFieldAmount    = "amount"
	eventFieldRequestID = "request_id"
)

// ScoreEvent is one entry of the ledger.
// Uses the default Redis stream entry id.
type ScoreEvent struct {
	ID        string    // assigned by Redis on append
	Type      EventType //
	PlayerID  string    //
	Amount    float64   //
	RequestID string    // client-supplied idempotency key
	At        time.Time // derived from the ms part of ID
}
