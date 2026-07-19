package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
)

// Present flags distinguish "wrong score" from "missing on one side".
type ScoreMismatch struct {
	BoardID       string
	PlayerID      string
	LiveScore     float64
	LivePresent   bool
	ReplayScore   float64
	ReplayPresent bool
}

// per-board ZSET scratch: transient rebuild target for VerifyProjection.
// Board id contains no ':' so this can never collide with another board.
func boardVerifyKey(id board.ID) string {
	return leaderboardKey(id) + ":tmp:verify"
}

// Drops every leaderboard projection and replays the whole ledger into them.
func (rs *redisStorage) ReplayLedger(ctx context.Context) error {
	events, err := rs.readLedger(ctx)
	if err != nil {
		return err
	}
	boards, err := rs.affectedBoards(ctx, events)
	if err != nil {
		return err
	}
	return rs.foldInto(ctx, events, boards, leaderboardKey)
}

// Reads the whole ledger, oldest first.
// MVP: one XRANGE reads the whole stream; pagination with COUNT batches is deferred.
func (rs *redisStorage) readLedger(ctx context.Context) ([]ledger.Event, error) {
	entries, err := rs.client.XRange(ctx, ledgerKey, "-", "+").Result()
	if err != nil {
		return nil, fmt.Errorf("storage rebuild: read ledger: %w", err)
	}
	events := make([]ledger.Event, 0, len(entries))
	for _, entry := range entries {
		events = append(events, entryToEvent(entry))
	}
	return events, nil
}

func (rs *redisStorage) affectedBoards(ctx context.Context, events []ledger.Event) ([]board.ID, error) {
	seen := make(map[board.ID]bool)
	out := make([]board.ID, 0)

	registered, err := rs.client.ZRange(ctx, boardsRegistryKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("storage rebuild: read registry: %w", err)
	}
	for _, boardId := range registered {
		if !seen[board.ID(boardId)] {
			seen[board.ID(boardId)] = true
			out = append(out, board.ID(boardId))
		}
	}
	for _, e := range events {
		if !seen[board.ID(e.BoardID)] {
			seen[board.ID(e.BoardID)] = true
			out = append(out, board.ID(e.BoardID))
		}
	}
	return out, nil
}

// Drops keyFor(board) for every affected board, then folds the events forward:
// `set` assigns the absolute score, `increment` adds the delta.
func (rs *redisStorage) foldInto(
	ctx context.Context, events []ledger.Event, boards []board.ID, keyFor func(board.ID) string,
) error {
	for _, b := range boards {
		if err := rs.client.Del(ctx, keyFor(b)).Err(); err != nil {
			return fmt.Errorf("storage rebuild: drop %q: %w", keyFor(b), err)
		}
	}

	for _, event := range events {
		destKey := keyFor(board.ID(event.BoardID))
		var err error
		switch event.Type {
		case ledger.EventSet:
			err = rs.client.ZAdd(ctx, destKey, redis.Z{Score: event.Amount, Member: event.PlayerID}).Err()
		case ledger.EventIncrement:
			err = rs.client.ZIncrBy(ctx, destKey, event.Amount, event.PlayerID).Err()
		}
		if err != nil {
			return fmt.Errorf("storage rebuild: apply event %s: %w", event.ID, err)
		}
	}
	return nil
}

// VerifyProjection replays the ledger into per-board scratch keys and diffs every
// affected board against its live projection. Empty result means no drift.
func (rs *redisStorage) VerifyProjection(ctx context.Context) ([]ScoreMismatch, error) {
	events, err := rs.readLedger(ctx)
	if err != nil {
		return nil, err
	}
	boards, err := rs.affectedBoards(ctx, events)
	if err != nil {
		return nil, err
	}

	if err := rs.foldInto(ctx, events, boards, boardVerifyKey); err != nil {
		return nil, err
	}
	// best-effort cleanup with a fresh context so a cancelled ctx doesn't leak scratch keys
	defer func() {
		for _, b := range boards {
			rs.client.Del(context.Background(), boardVerifyKey(b))
		}
	}()

	var mismatches []ScoreMismatch
	for _, b := range boards {
		boardMismatches, err := rs.diffBoard(ctx, b)
		if err != nil {
			return nil, err
		}
		mismatches = append(mismatches, boardMismatches...)
	}
	return mismatches, nil
}

func (rs *redisStorage) diffBoard(ctx context.Context, b board.ID) ([]ScoreMismatch, error) {
	live, err := rs.zsetScores(ctx, leaderboardKey(b))
	if err != nil {
		return nil, err
	}
	rebuilt, err := rs.zsetScores(ctx, boardVerifyKey(b))
	if err != nil {
		return nil, err
	}

	var mismatches []ScoreMismatch
	// scores go through identical Redis float ops in identical order on both paths, so an
	// exact comparison is ok here
	for id, liveScore := range live {
		rebuiltScore, ok := rebuilt[id]
		if !ok || rebuiltScore != liveScore {
			mismatches = append(mismatches, ScoreMismatch{
				BoardID:  string(b),
				PlayerID: id, LiveScore: liveScore, LivePresent: true,
				ReplayScore: rebuiltScore, ReplayPresent: ok,
			})
		}
	}
	for id, rebuiltScore := range rebuilt {
		if _, ok := live[id]; !ok {
			mismatches = append(mismatches, ScoreMismatch{
				BoardID:  string(b),
				PlayerID: id, LivePresent: false,
				ReplayScore: rebuiltScore, ReplayPresent: true,
			})
		}
	}
	return mismatches, nil
}

// Reads a whole ZSET into a member->score map.
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
