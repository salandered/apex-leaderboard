package storage

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/ledger"
)

var (
	StorageError     = errors.New("storage")
	ErrNotFound      = fmt.Errorf("%w: not found", StorageError)
	ErrInconsistent  = fmt.Errorf("%w: inconsistent", StorageError)
	ErrPlayerExists  = fmt.Errorf("%w: player exists", StorageError)
	ErrBoardExists   = fmt.Errorf("%w: board exists", StorageError)
	ErrBoardNotFound = fmt.Errorf("%w: board not found", StorageError)
	ErrBoardClosed   = fmt.Errorf("%w: board closed", StorageError)
)

type redisStorage struct {
	client *redis.Client
}

// Field keys stored in each ledger:events entry.
// Must match the Lua write script.
const (
	entryFieldType      = "type"
	entryFieldPlayerID  = "player_id"
	entryFieldBoardID   = "board_id"
	entryFieldAmount    = "amount"
	entryFieldRequestID = "request_id"
)

// A malformed amount (should never happen: the write script always stores a number) decodes to 0.
func entryToEvent(entry redis.XMessage) ledger.Event {
	amount, _ := strconv.ParseFloat(getStreamEntryValue(entry, entryFieldAmount), 64)
	return ledger.Event{
		ID:        entry.ID,
		Type:      ledger.EventType(getStreamEntryValue(entry, entryFieldType)),
		PlayerID:  getStreamEntryValue(entry, entryFieldPlayerID),
		BoardID:   getStreamEntryValue(entry, entryFieldBoardID),
		Amount:    amount,
		RequestID: getStreamEntryValue(entry, entryFieldRequestID),
		CreatedAt: eventTime(entry.ID),
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
	t, err := apextime.FromStreamID(id)
	if err != nil {
		return time.Time{}
	}
	return t
}
