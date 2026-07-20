package storage

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

const (
	ledgerKey      = "ledger:events"      // STREAM: the ledger
	idempotencyKey = "ledger:idempotency" // HASH {board_id}:{player_id}:{request_id} -> stream id: replays the original result on retry
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

//go:embed scripts/apply_score_event.lua
var applyScoreLua string

var applyScoreScript = redis.NewScript(applyScoreLua)

// per-board ZSET projection
func leaderboardKey(id board.ID) string {
	return "leaderboard:" + string(id)
}

// Sets an absolute score.
// An unknown player/board returns ErrNotFound/ErrBoardNotFound without appending.
func (rs *redisStorage) SetScore(ctx context.Context, playerId player.ID, boardId board.ID, score float64, requestID string) error {
	_, err := rs.applyEvent(ctx, ledger.EventSet, playerId, boardId, score, requestID)
	return err
}

// Applies a delta to score on the board (a player with no entry starts from 0).
// An unknown player/board returns ErrNotFound/ErrBoardNotFound without appending.
func (rs *redisStorage) IncrementScore(ctx context.Context, playerId player.ID, boardId board.ID, amount float64, requestID string) (float64, error) {
	return rs.applyEvent(ctx, ledger.EventIncrement, playerId, boardId, amount, requestID)
}

// Commands are pipelined into one round trip (best-effort, not atomic: consider using MULT).
func (rs *redisStorage) GetStanding(ctx context.Context, playerId player.ID, boardId board.ID) (Standing, int64, error) {
	pipe := rs.client.Pipeline()
	rankCmd := pipe.ZRevRankWithScore(ctx, leaderboardKey(boardId), string(playerId)) // O(log(N))
	cardCmd := pipe.ZCard(ctx, leaderboardKey(boardId))                               // O(1)

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return Standing{}, 0, fmt.Errorf("storage get standing: %w", err)
	}

	rankScore, err := rankCmd.Result()
	// ZREVRANK (ZRevRankWithScore) returns redis.Nil on a missing member
	if errors.Is(err, redis.Nil) {
		return Standing{}, 0, ErrNotFound
	}
	if err != nil {
		return Standing{}, 0, fmt.Errorf("storage get standing: %w", err)
	}
	total, err := cardCmd.Result()
	if err != nil {
		return Standing{}, 0, fmt.Errorf("storage get standing: %w", err)
	}

	standing := Standing{
		PlayerID: string(playerId),
		Score:    rankScore.Score,
		Rank:     rankScore.Rank + 1, // ZREVRANK is 0-based
	}
	return standing, total, nil
}

// An offset past the end yields an empty (non-nil) slice; total is still the full board size.
// An unknown board reads as an empty board, not an error currently.
func (rs *redisStorage) ListStandings(
	ctx context.Context, boardId board.ID, limit, offset int64,
) ([]Standing, int64, error) {
	// Guard limit: ZREVRANGE with stop == -1 wraps to the last element and would return the
	// whole board. Callers validate limit >= 1; this is defence in depth.
	if limit <= 0 {
		total, err := rs.client.ZCard(ctx, leaderboardKey(boardId)).Result()
		if err != nil {
			return nil, 0, fmt.Errorf("storage list scores: count: %w", err)
		}
		return []Standing{}, total, nil
	}

	// rank is 1-based and continues across pages: the row at result index i has rank offset+i+1.
	stop := offset + limit - 1
	pipe := rs.client.Pipeline()
	cardCmd := pipe.ZCard(ctx, leaderboardKey(boardId))                              // O(1)
	rangeCmd := pipe.ZRevRangeWithScores(ctx, leaderboardKey(boardId), offset, stop) // O(log N + page)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("storage list scores: %w", err)
	}

	// err checks here are redundant (after a strict Exec check)
	// kept for uniformity — see docs/redis.md
	total, err := cardCmd.Result()
	if err != nil {
		return nil, 0, fmt.Errorf("storage list scores: count: %w", err)
	}
	zs, err := rangeCmd.Result()
	if err != nil {
		return nil, 0, fmt.Errorf("storage list scores: range: %w", err)
	}
	out := make([]Standing, 0, len(zs))
	for i, z := range zs {
		out = append(out, Standing{
			PlayerID: z.Member.(string),
			Score:    z.Score,
			Rank:     offset + int64(i) + 1,
		})
	}
	return out, total, nil
}

// TODO we scan the whole stream (XREVRANGE + -) and filter by player_id, no pagination
func (rs *redisStorage) PlayerHistory(
	ctx context.Context, playerId player.ID, boardId board.ID, limit int64,
) ([]ledger.Event, error) {
	entries, err := rs.client.XRevRange(ctx, ledgerKey, "+", "-").Result()
	if err != nil {
		return nil, fmt.Errorf("storage history: %w", err)
	}

	events := make([]ledger.Event, 0)
	for _, entry := range entries {
		if getStreamEntryValue(entry, entryFieldPlayerID) != string(playerId) ||
			getStreamEntryValue(entry, entryFieldBoardID) != string(boardId) {
			continue
		}
		event, err := entryToEvent(entry)
		if err != nil {
			return nil, fmt.Errorf("storage history: %w", err)
		}
		events = append(events, event)
		if limit > 0 && int64(len(events)) >= limit {
			break
		}
	}
	return events, nil
}

// Script result codes. Must match the Lua write script.
const (
	applyCodeApplied        = 1  // event appended, projection updated
	applyCodeDeduped        = 0  // request_id seen before: original result returned
	applyCodePlayerNotFound = -1 // player hash missing
	applyCodeBoardNotFound  = -2 // board hash missing
	applyCodeBoardClosed    = -3 // board state is "closed": writes rejected
)

// Runs the write script and returns the player's score on the board.
// A rejected write appends nothing and maps to an error.
func (rs *redisStorage) applyEvent(
	ctx context.Context,
	etype ledger.EventType,
	playerId player.ID,
	boardId board.ID,
	amount float64,
	requestID string,
) (float64, error) {
	result, err := applyScoreScript.Run(ctx, rs.client,
		[]string{leaderboardKey(boardId), ledgerKey, idempotencyKey, playerHashKey(playerId), boardHashKey(boardId)},
		string(etype), string(playerId), amount, requestID, string(boardId),
	).Slice()
	if err != nil {
		return 0, fmt.Errorf("storage apply %s event: %w", etype, err)
	}
	// result = { code(int64), new_score(string), stream_id(string) }
	switch code := result[0].(int64); code {
	case applyCodeApplied, applyCodeDeduped:
		// both ok
	case applyCodePlayerNotFound:
		return 0, ErrNotFound
	case applyCodeBoardNotFound:
		return 0, ErrBoardNotFound
	case applyCodeBoardClosed:
		return 0, ErrBoardClosed
	default:
		return 0, fmt.Errorf("storage apply %s event: unexpected script code %d", etype, code)
	}
	score, err := strconv.ParseFloat(result[1].(string), 64)
	if err != nil {
		return 0, fmt.Errorf("storage apply %s event: parse score %q: %w", etype, result[1], err)
	}
	return score, nil
}
