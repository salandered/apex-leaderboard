package storage

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

// Both halves of a "<millis>-<seq>" stream id carry order, and a ZSET breaks score ties by member
// lexicographically ("10" before "2"). So the score packs both. Stays exact in a float64 (under
// 2^53) until millis ~2.19e12, year 2039.
const seqScale = 4096

type redisPlayerHistoryStore struct {
	redisLedgerConsumer
}

func NewPlayerHistoryStore(client *redis.Client) *redisPlayerHistoryStore {
	return &redisPlayerHistoryStore{redisLedgerConsumer{client: client}}
}

// Writes pointers to events in the ledger to ZSET.
// ZSET's member is the entry id and the score derives from it.
// Replayed batch rewrites the same pairs.
func (s *redisPlayerHistoryStore) ApplyPlayerHistory(
	ctx context.Context, events []ledger.Event,
) error {
	pipe := s.client.Pipeline()
	for _, event := range events {
		score, err := streamIDScore(ctx, event.ID)
		if err != nil {
			return fmt.Errorf("storage apply player history: %w", err)
		}
		pipe.ZAdd(ctx,
			playerHistoryKey(player.ID(event.PlayerID), board.ID(event.BoardID)),
			redis.Z{Score: score, Member: event.ID},
		)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("storage apply player history: %w", err)
	}
	return nil
}

// streamIDScore turns a Redis stream entry id into the ZSET score.
// See unit test for examples.
func streamIDScore(ctx context.Context, entryID string) (float64, error) {
	millisStr, seqStr, ok := strings.Cut(entryID, "-")
	if !ok {
		return 0, fmt.Errorf("%w: stream id '%s' has no sequence part", ErrInconsistent, entryID)
	}
	millis, err := strconv.ParseInt(millisStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: stream id '%s' has a bad millisecond part", ErrInconsistent, entryID)
	}
	seq, err := strconv.ParseInt(seqStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: stream id '%s' has a bad sequence part", ErrInconsistent, entryID)
	}
	// Needs four million writes a second, well past a single redis node. Clamped, not rejected: an
	// absurd ledger should misorder one millisecond, not wedge the consumer on a batch forever.
	if seq >= seqScale {
		slog.WarnContext(ctx, "stream id sequence exceeds the index scale, clamping",
			"component", logComponent, "entry_id", entryID, "seq_scale", seqScale)
		seq = seqScale - 1
	}
	return float64(millis*seqScale + seq), nil
}
