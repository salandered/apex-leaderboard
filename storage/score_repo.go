package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

// Validates existence first and return ErrNotFound without appending.
func (rs *redisStorage) SetScore(ctx context.Context, playerId player.ID, boardId board.ID, score float64, requestID string) error {
	if err := rs.requireExists(ctx, playerId, boardId); err != nil {
		return err
	}
	_, err := rs.applyEvent(ctx, ledger.EventSet, playerId, score, requestID)
	return err
}

// Validates existence first and return ErrNotFound without appending.
func (rs *redisStorage) IncrementScore(ctx context.Context, playerId player.ID, boardId board.ID, amount float64, requestID string) (float64, error) {
	if err := rs.requireExists(ctx, playerId, boardId); err != nil {
		return 0, err
	}
	return rs.applyEvent(ctx, ledger.EventIncrement, playerId, amount, requestID)
}

// Commands are pipelined into one round trip (best-effort, not atomic: consider using MULT).
func (rs *redisStorage) GetStanding(ctx context.Context, playerId player.ID, boardId board.ID) (Standing, int64, error) {
	pipe := rs.client.Pipeline()
	rankCmd := pipe.ZRevRankWithScore(ctx, leaderboardKey, string(playerId)) // O(log(N))
	cardCmd := pipe.ZCard(ctx, leaderboardKey)                               // O(1)

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
func (rs *redisStorage) ListStandings(
	ctx context.Context, boardId board.ID, limit, offset int64,
) ([]Standing, int64, error) {
	// Guard limit: ZREVRANGE with stop == -1 wraps to the last element and would return the
	// whole board. Callers validate limit >= 1; this is defence in depth.
	if limit <= 0 {
		total, err := rs.client.ZCard(ctx, leaderboardKey).Result()
		if err != nil {
			return nil, 0, fmt.Errorf("storage list scores: count: %w", err)
		}
		return []Standing{}, total, nil
	}

	// rank is 1-based and continues across pages: the row at result index i has rank offset+i+1.
	stop := offset + limit - 1
	pipe := rs.client.Pipeline()
	cardCmd := pipe.ZCard(ctx, leaderboardKey)                              // O(1)
	rangeCmd := pipe.ZRevRangeWithScores(ctx, leaderboardKey, offset, stop) // O(log N + page)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("storage list scores: %w", err)
	}

	// err checks here are redundant (after a strict Exec check)
	// , kept for uniformity — see docs/redis.md
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
		if getStreamEntryValue(entry, entryFieldPlayerID) != string(playerId) {
			continue
		}
		events = append(events, entryToEvent(entry))
		if limit > 0 && int64(len(events)) >= limit {
			break
		}
	}
	return events, nil
}

// returns error on unknown board or player; note that a player without a score on the board is fine
// TODO: only the seeded main board exists until board CRUD lands.
func (rs *redisStorage) requireExists(ctx context.Context, playerId player.ID, boardId board.ID) error {
	if boardId != board.MainId {
		return ErrNotFound
	}
	n, err := rs.client.Exists(ctx, playerHashKey(playerId)).Result() // O(number of key)
	if err != nil {
		return fmt.Errorf("storage check player exists: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
