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

	// History reads the ledger (newest first) for one player, up to limit entries
	// (limit <= 0 means no cap).
	History(ctx context.Context, playerId player.ID, limit int64) ([]ScoreEvent, error)

	// Rebuild drops the projection and replays the whole ledger into it. Ops/repair
	// tool: the projection is disposable and reconstructible from the source of truth.
	Rebuild(ctx context.Context) error

	// VerifyProjection replays the ledger into a scratch key and reports any player
	// whose live score disagrees with the replay. Empty result == projection matches
	// the ledger (proof no write bypassed the script). Cheap consistency self-check.
	VerifyProjection(ctx context.Context) ([]ScoreMismatch, error)
}

const (
	leaderboardKey  = "leaderboard"        // ZSET projection: the ranking index
	verifyKey       = "leaderboard:verify" // ZSET scratch: transient rebuild for VerifyProjection
	streamKey       = "score:events"       // STREAM: the ledger, source of truth
	appliedKey      = "score:applied"      // HASH request_id -> stream id: idempotency table
	playerNameField = "player_name"
)

// ScoreMismatch is one player whose live projection score differs from the ledger replay.
// Present flags distinguish "wrong score" from "missing on one side".
type ScoreMismatch struct {
	PlayerID       string
	LiveScore      float64
	LivePresent    bool
	RebuiltScore   float64
	RebuiltPresent bool
}

// The write path for score mutations. Every increment/set goes through this script so the
// projection (leaderboard) and the ledger (score:events) move together atomically. See the
// script header for the KEYS/ARGV contract.
//
//go:embed scripts/apply_score_event.lua
var applyScoreScript string

var applyScore = redis.NewScript(applyScoreScript)

// The create path: profile hash + initial score + first event in one atomic script, so a
// profile can never exist without its score (no dual write). Sibling of applyScore.
//
//go:embed scripts/create_player.lua
var createPlayerScript string

var createPlayer = redis.NewScript(createPlayerScript)

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

// Writes the profile (Hash), the initial score, and the first `set` event in one atomic
// script, so a profile can never exist without its score. Unconditional create — the
// player doesn't exist in the projection yet, so there's no pre-check.
func (rs *redisStorage) CreatePlayer(
	ctx context.Context,
	profile *player.Profile,
	score float64,
	requestID string,
) error {
	err := createPlayer.Run(
		ctx,
		rs.client,
		[]string{leaderboardKey, streamKey, appliedKey, playerHashKey(profile.PlayerId)},
		string(profile.PlayerId), profile.PlayerName, score, requestID,
	).Err()
	if err != nil {
		return fmt.Errorf("storage create player: %w", err)
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

// History returns one player's ledger entries, newest first.
//
// MVP: scan the whole stream (XREVRANGE + -) and filter by player_id. This is
// O(stream length) per call — acceptable at prototype scale; Step 5 replaces it with a
// cursor or a per-player stream. An unknown player simply yields an empty slice.
func (rs *redisStorage) History(ctx context.Context, playerId player.ID, limit int64) ([]ScoreEvent, error) {
	entries, err := rs.client.XRevRange(ctx, streamKey, "+", "-").Result()
	if err != nil {
		return nil, fmt.Errorf("storage history: %w", err)
	}

	events := make([]ScoreEvent, 0)
	for _, entry := range entries {
		if getStreamEntryValue(entry, eventFieldPlayerID) != string(playerId) {
			continue
		}
		events = append(events, toScoreEvent(entry))
		if limit > 0 && int64(len(events)) >= limit {
			break
		}
	}
	return events, nil
}

// Rebuild drops the live projection and replays the whole ledger into it.
func (rs *redisStorage) Rebuild(ctx context.Context) error {
	return rs.rebuildInto(ctx, leaderboardKey)
}

// rebuildInto drops destKey and folds the ledger forward into it: `set` assigns the
// absolute score, `increment` adds the delta. Because replay applies the same ops in the
// same order as the live write path, the result is identical to the live projection.
//
// MVP: one XRANGE reads the whole stream. Step 5 paginates with COUNT batches + pipelining.
func (rs *redisStorage) rebuildInto(ctx context.Context, destKey string) error {
	if err := rs.client.Del(ctx, destKey).Err(); err != nil {
		return fmt.Errorf("storage rebuild: drop %q: %w", destKey, err)
	}

	entries, err := rs.client.XRange(ctx, streamKey, "-", "+").Result()
	if err != nil {
		return fmt.Errorf("storage rebuild: read ledger: %w", err)
	}
	for _, entry := range entries {
		event := toScoreEvent(entry)
		switch event.Type {
		case EventSet:
			err = rs.client.ZAdd(ctx, destKey, redis.Z{Score: event.Amount, Member: event.PlayerID}).Err()
		case EventIncrement:
			err = rs.client.ZIncrBy(ctx, destKey, event.Amount, event.PlayerID).Err()
		}
		if err != nil {
			return fmt.Errorf("storage rebuild: apply event %s: %w", event.ID, err)
		}
	}
	return nil
}

// VerifyProjection rebuilds the ledger into a scratch key and diffs it against the live
// projection. An empty result proves the live projection matches the ledger.
func (rs *redisStorage) VerifyProjection(ctx context.Context) ([]ScoreMismatch, error) {
	if err := rs.rebuildInto(ctx, verifyKey); err != nil {
		return nil, err
	}
	// best-effort cleanup with a fresh context so a cancelled ctx doesn't leak the scratch key
	defer rs.client.Del(context.Background(), verifyKey)

	live, err := rs.zsetScores(ctx, leaderboardKey)
	if err != nil {
		return nil, err
	}
	rebuilt, err := rs.zsetScores(ctx, verifyKey)
	if err != nil {
		return nil, err
	}

	var mismatches []ScoreMismatch
	// scores go through identical Redis float ops in identical order on both paths, so an
	// exact comparison is correct here — no epsilon needed.
	for id, liveScore := range live {
		rebuiltScore, ok := rebuilt[id]
		if !ok || rebuiltScore != liveScore {
			mismatches = append(mismatches, ScoreMismatch{
				PlayerID: id, LiveScore: liveScore, LivePresent: true,
				RebuiltScore: rebuiltScore, RebuiltPresent: ok,
			})
		}
	}
	for id, rebuiltScore := range rebuilt {
		if _, ok := live[id]; !ok {
			mismatches = append(mismatches, ScoreMismatch{
				PlayerID: id, LivePresent: false,
				RebuiltScore: rebuiltScore, RebuiltPresent: true,
			})
		}
	}
	return mismatches, nil
}

// zsetScores reads a whole ZSET into a member->score map.
func (rs *redisStorage) zsetScores(ctx context.Context, key string) (map[string]float64, error) {
	zs, err := rs.client.ZRangeWithScores(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("storage read zset %q: %w", key, err)
	}
	out := make(map[string]float64, len(zs))
	for _, z := range zs {
		out[z.Member.(string)] = z.Score
	}
	return out, nil
}
