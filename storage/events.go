package storage

import (
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// EventType enumerates the operations that can appear in the ledger.
type EventType string

const (
	EventSet       EventType = "set"       // Amount is the absolute value
	EventIncrement EventType = "increment" // Amount is the delta (may be negative)
)

// Stream event fields. These are the field keys stored in each score:events
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

// toScoreEvent decodes a raw stream entry into a ScoreEvent. A malformed amount
// (should never happen: the write script always stores a number) decodes to 0.
func toScoreEvent(entry redis.XMessage) ScoreEvent {
	amount, _ := strconv.ParseFloat(getStreamEntryValue(entry, eventFieldAmount), 64)
	return ScoreEvent{
		ID:        entry.ID,
		Type:      EventType(getStreamEntryValue(entry, eventFieldType)),
		PlayerID:  getStreamEntryValue(entry, eventFieldPlayerID),
		Amount:    amount,
		RequestID: getStreamEntryValue(entry, eventFieldRequestID),
		At:        eventTime(entry.ID),
	}
}

// Reads a string field from a stream entry's values, "" if absent.
func getStreamEntryValue(entry redis.XMessage, key string) string {
	if v, ok := entry.Values[key].(string); ok {
		return v
	}
	return ""
}

// eventTime derives the entry timestamp from the ms part of a Redis stream id
// ("<unix_ms>-<seq>"). A malformed id yields the zero time.
func eventTime(id string) time.Time {
	msPart, _, _ := strings.Cut(id, "-")
	ms, err := strconv.ParseInt(msPart, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
