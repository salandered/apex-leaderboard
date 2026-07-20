package storage

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

var (
	StorageError     = errors.New("storage")
	ErrNotFound      = fmt.Errorf("%w: not found", StorageError)
	ErrInconsistent  = fmt.Errorf("%w: inconsistent", StorageError)
	ErrPlayerExists  = fmt.Errorf("%w: player exists", StorageError)
	ErrBoardExists   = fmt.Errorf("%w: board exists", StorageError)
	ErrBoardNotFound = fmt.Errorf("%w: board not found", StorageError)
	ErrBoardClosed   = fmt.Errorf("%w: board closed", StorageError)
	// An idempotency key reused with a different operation or payload.
	ErrIdempotencyConflict = fmt.Errorf("%w: idempotency conflict", StorageError)
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

func entryToEvent(entry redis.XMessage) (ledger.Event, error) {
	required := func(field string) (string, error) {
		value := getStreamEntryValue(entry, field)
		if value == "" {
			return "", fmt.Errorf(
				"%w: ledger event '%s' missing field '%s'", ErrInconsistent, entry.ID, field,
			)
		}
		return value, nil
	}

	rawType, err := required(entryFieldType)
	if err != nil {
		return ledger.Event{}, err
	}
	eventType := ledger.EventType(rawType)
	if eventType != ledger.EventSet && eventType != ledger.EventIncrement {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has unknown type %q", ErrInconsistent, entry.ID, rawType,
		)
	}
	playerId, err := required(entryFieldPlayerID)
	if err != nil {
		return ledger.Event{}, err
	}
	if err := player.ID(playerId).Validate(); err != nil {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has invalid player id: %v", ErrInconsistent, entry.ID, err,
		)
	}
	boardId, err := required(entryFieldBoardID)
	if err != nil {
		return ledger.Event{}, err
	}
	rawAmount, err := required(entryFieldAmount)
	if err != nil {
		return ledger.Event{}, err
	}
	amount, err := strconv.ParseFloat(rawAmount, 64)
	if err != nil || math.IsNaN(amount) {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has invalid amount %q", ErrInconsistent, entry.ID, rawAmount,
		)
	}
	requestId, err := required(entryFieldRequestID)
	if err != nil {
		return ledger.Event{}, err
	}

	return ledger.Event{
		ID:        entry.ID,
		Type:      eventType,
		PlayerID:  playerId,
		BoardID:   boardId,
		Amount:    amount,
		RequestID: requestId,
		CreatedAt: eventTime(entry.ID),
	}, nil
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
