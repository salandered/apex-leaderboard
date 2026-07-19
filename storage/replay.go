package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/ledger"
)

// Present flags distinguish "wrong score" from "missing on one side".
type ScoreMismatch struct {
	PlayerID      string
	LiveScore     float64
	LivePresent   bool
	ReplayScore   float64
	ReplayPresent bool
}

// Drops the live projection and replays the whole ledger into it.
func (rs *redisStorage) ReplayLedger(ctx context.Context) error {
	return rs.rebuildInto(ctx, leaderboardKey)
}

// Drops destKey and folds the ledger forward into it: `set` assigns the
// absolute score, `increment` adds the delta. Because replay applies the same ops in the
// same order as the live write path, the result is identical to the live projection.
//
// MVP: one XRANGE reads the whole stream. Step 5 paginates with COUNT batches + pipelining.
func (rs *redisStorage) rebuildInto(ctx context.Context, destKey string) error {
	if err := rs.client.Del(ctx, destKey).Err(); err != nil {
		return fmt.Errorf("storage rebuild: drop %q: %w", destKey, err)
	}

	entries, err := rs.client.XRange(ctx, ledgerKey, "-", "+").Result()
	if err != nil {
		return fmt.Errorf("storage rebuild: read ledger: %w", err)
	}
	for _, entry := range entries {
		event := entryToEvent(entry)
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
				ReplayScore: rebuiltScore, ReplayPresent: ok,
			})
		}
	}
	for id, rebuiltScore := range rebuilt {
		if _, ok := live[id]; !ok {
			mismatches = append(mismatches, ScoreMismatch{
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
