package storage

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

var (
	StorageError    = errors.New("storage")
	ErrNotFound     = fmt.Errorf("%w.not found", StorageError)
	ErrInconsistent = fmt.Errorf("%w.inconsistent", StorageError)
)

const (
	leaderboardKey = "leaderboard"            // ZSET projection: the ranking index
	verifyKey      = "leaderboard:tmp:verify" // ZSET scratch: transient rebuild for VerifyProjection
	ledgerKey      = "score:events"           // STREAM: the ledger, source of truth
	appliedKey     = "score:applied"          // HASH request_id -> stream id: idempotency table

)

// A player's current standing in the projection.
// Rank is 1-based (rank 1 means highest score).
// Formerly known as RankedScore.
type Standing struct {
	// consider moving out of storage
	PlayerID string
	Score    float64
	Rank     int64
}

// The write path for score mutations. Every increment/set goes through this script so the
// projection (leaderboard) and the ledger (score:events) move together atomically.
//
//go:embed scripts/apply_score_event.lua
var applyScoreLua string

var applyScoreScript = redis.NewScript(applyScoreLua)

type redisStorage struct {
	client *redis.Client
}

// Field keys stored in each score:events entry.
// Must match the Lua write script.
const (
	entryFieldType      = "type"
	entryFieldPlayerID  = "player_id"
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

// Runs the write script and returns the player's score.
// Callers do request validation first; by here the write is accepted.
func (rs *redisStorage) applyEvent(ctx context.Context, etype ledger.EventType, playerId player.ID, amount float64, requestID string) (float64, error) {
	res, err := applyScoreScript.Run(ctx, rs.client,
		[]string{leaderboardKey, ledgerKey, appliedKey},
		string(etype), string(playerId), amount, requestID,
	).Slice()
	if err != nil {
		return 0, fmt.Errorf("storage apply %s event: %w", etype, err)
	}
	// res = { applied(int64), new_score(string), stream_id(string) }
	score, err := strconv.ParseFloat(res[1].(string), 64)
	if err != nil {
		return 0, fmt.Errorf("storage apply %s event: parse score %q: %w", etype, res[1], err)
	}
	return score, nil
}
