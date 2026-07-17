package storage

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/player"
)

var (
	StorageError    = errors.New("storage")
	ErrNotFound     = fmt.Errorf("%w.not found", StorageError)
	ErrInconsistent = fmt.Errorf("%w.inconsistent", StorageError)
)

// requestID is a client-supplied idempotency key: replaying the same key is a no-op,
// making the operation idempotent.
type Storage interface {
	CreatePlayer(ctx context.Context, profile *player.Profile, score float64, requestID string) error
	GetPlayer(ctx context.Context, playerId player.ID) (*player.Profile, float64, error)
	IncrementScore(ctx context.Context, playerId player.ID, amount float64, requestID string) (float64, error)
	SetScore(ctx context.Context, playerId player.ID, score float64, requestID string) error
}

const (
	leaderboardKey  = "leaderboard"   // ZSET projection: the ranking index
	streamKey       = "score:events"  // STREAM: the ledger, source of truth
	appliedKey      = "score:applied" // HASH request_id -> stream id: idempotency table
	playerNameField = "player_name"
)

// The single write path. Every score change goes through this script so the projection
// (leaderboard) and the ledger (score:events) move together atomically. See the script
// header for the KEYS/ARGV contract.
//
//go:embed scripts/apply_score_event.lua
var applyScoreScript string

var applyScore = redis.NewScript(applyScoreScript)

type redisStorage struct {
	client *redis.Client
}

// Runs the write script and returns the player's score.
// Callers do request validation first; by here the write is accepted.
func (rs *redisStorage) applyEvent(ctx context.Context, etype EventType, playerId player.ID, amount float64, requestID string) (float64, error) {
	res, err := applyScore.Run(ctx, rs.client,
		[]string{leaderboardKey, streamKey, appliedKey},
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

// builds the profile Hash key
func playerHashKey(id player.ID) string {
	return "player:" + string(id)
}

// writes the profile (Hash) and the initial score as a `set` event through the ledger.
// The two aren't in one atomic unit: the profile Hash isn't a key of the write script.
// The window is small and create-time only; if the score write fails after the Hash is
// written, GetPlayer reports ErrInconsistent (profile without score). A future
// create-specific script could fold both if this ever matters.
func (rs *redisStorage) CreatePlayer(ctx context.Context, profile *player.Profile, score float64, requestID string) error {
	err := rs.client.HSet(ctx, playerHashKey(profile.PlayerId), playerNameField, profile.PlayerName).Err()
	if err != nil {
		return fmt.Errorf("storage create profile: %w", err)
	}
	// unconditional set: the player doesn't exist in the projection yet, so no pre-check.
	if _, err := rs.applyEvent(ctx, EventSet, profile.PlayerId, score, requestID); err != nil {
		return err
	}
	return nil
}

// reads the profile (Hash) and score (Sorted Set) for a player
func (rs *redisStorage) GetPlayer(ctx context.Context, playerId player.ID) (*player.Profile, float64, error) {
	fields, err := rs.client.HGetAll(ctx, playerHashKey(playerId)).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("storage get profile: %w", err)
	}
	if len(fields) == 0 {
		return nil, 0, ErrNotFound
	}

	score, err := rs.client.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	if errors.Is(err, redis.Nil) {
		// consistency problem: hash exists but its sorted-set score is missing
		return nil, 0, fmt.Errorf("%w: player '%s' has profile but no score", ErrInconsistent, playerId)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("storage get score: %w", err)
	}

	name, ok := fields[playerNameField]
	if !ok {
		// consistency problem: CreatePlayer writes the hash and sorted-set entry atomically
		return nil, 0, fmt.Errorf("%w: player '%s' hash missing field '%s'", ErrInconsistent, playerId, playerNameField)
	}

	profile := &player.Profile{
		PlayerId:   playerId,
		PlayerName: name,
	}
	return profile, score, nil
}

// Applies a delta to an existing player's score.
// Validates existence first and return ErrNotFound without appending.
func (rs *redisStorage) IncrementScore(ctx context.Context, playerId player.ID, amount float64, requestID string) (float64, error) {
	if err := rs.requireExists(ctx, playerId); err != nil {
		return 0, err
	}
	return rs.applyEvent(ctx, EventIncrement, playerId, amount, requestID)
}

// Sets an absolute to an existing player's score.
// Validates existence first and return ErrNotFound without appending.
func (rs *redisStorage) SetScore(ctx context.Context, playerId player.ID, score float64, requestID string) error {
	if err := rs.requireExists(ctx, playerId); err != nil {
		return err
	}
	_, err := rs.applyEvent(ctx, EventSet, playerId, score, requestID)
	return err
}

// requireExists returns ErrNotFound if the player has no score in the projection.
func (rs *redisStorage) requireExists(ctx context.Context, playerId player.ID) error {
	err := rs.client.ZScore(ctx, leaderboardKey, string(playerId)).Err()
	if errors.Is(err, redis.Nil) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("storage check player exists: %w", err)
	}
	return nil
}
