package storage

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/score"
)

var (
	ErrStorage       = errors.New("storage")
	ErrNotFound      = fmt.Errorf("%w: not found", ErrStorage)
	ErrInconsistent  = fmt.Errorf("%w: inconsistent", ErrStorage)
	ErrPlayerExists  = fmt.Errorf("%w: player exists", ErrStorage)
	ErrBoardExists   = fmt.Errorf("%w: board exists", ErrStorage)
	ErrBoardNotFound = fmt.Errorf("%w: board not found", ErrStorage)
	ErrBoardClosed   = fmt.Errorf("%w: board closed", ErrStorage)
	// An idempotency key reused with a different operation or payload.
	ErrIdempotencyConflict = fmt.Errorf("%w: idempotency conflict", ErrStorage)
	// The write would move the score outside [score.Min, score.Max]; nothing was appended.
	ErrScoreOutOfRange = fmt.Errorf("%w: score out of range", ErrStorage)
)

type redisStorage struct {
	client *redis.Client
}

func NewStorage(client *redis.Client) Storage {
	return &redisStorage{client: client}
}

func (rs *redisStorage) Ping(ctx context.Context) error {
	if err := rs.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("storage ping: %w", err)
	}
	return nil
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
			"%w: ledger event '%s' has invalid player id: %w", ErrInconsistent, entry.ID, err,
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
	amountFloat, err := strconv.ParseFloat(rawAmount, 64) // ParseInt ?
	if err != nil {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has invalid amount %q", ErrInconsistent, entry.ID, rawAmount,
		)
	}
	// Strict, unlike the projection reads.
	// Rounding here would rewrite history and a rebuild would reproduce the rounded version.
	amount, ok := exactInt64(amountFloat)
	if !ok {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has invalid amount %q", ErrInconsistent, entry.ID, rawAmount,
		)
	}
	// Very bad: the write path bounds every amount, so an out-of-range one cannot have been appended.
	if err := score.Validate(amount); err != nil {
		return ledger.Event{}, fmt.Errorf(
			"%w: ledger event '%s' has out of range amount: %w", ErrInconsistent, entry.ID, err,
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

// Scores from Redis as float64 (ZSET scores, and the ledger's decimal string).
// A value is ok if it is finite, integral and inside the int64 range.
func exactInt64(v float64) (int64, bool) {
	// 2^63 is exactly representable; math.MaxInt64 is not, so compare against the power of two
	const overInt64 = float64(1 << 63)
	if math.IsNaN(v) || v >= overInt64 || v < -overInt64 {
		return 0, false
	}
	if v != math.Trunc(v) {
		return 0, false
	}
	return int64(v), true
}

// Reads a projection (ZSET) score.
// Soft, unlike the ledger decode above: projections are disposable, and failing the read
// would let one drifted member break every page containing it.
// Returns false if int64 can't hold the value, the caller decides what to do.
func zScoreToInt64(ctx context.Context, raw float64, key, member string) (int64, bool) {
	v, ok := exactInt64(raw)
	if !ok {
		// NaN and ±Inf round to themselves, so exactInt64 still rejects them
		v, ok = exactInt64(math.Round(raw))
		if !ok {
			slog.ErrorContext(ctx, "projection score is not representable",
				"key", key, "member", member, "score", raw,
			)
			return 0, false
		}
		slog.WarnContext(ctx, "projection score is not integral, rounding",
			"key", key, "member", member, "score", raw, "rounded", v,
		)
	}
	// Out of range is drift the write path can no longer produce, but the value still serves
	// fine, so report it instead of dropping the row.
	if score.Validate(v) != nil {
		slog.WarnContext(ctx, "projection score is out of range", "key", key, "member", member, "score", v)
	}
	return v, true
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
