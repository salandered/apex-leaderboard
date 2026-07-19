package storage

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
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

// Drops one board's projection and rebuilds it from its events in the global ledger.
func (rs *redisStorage) RebuildProjection(ctx context.Context, boardId board.ID) error {
	if err := rs.requireBoard(ctx, boardId); err != nil {
		return err
	}
	events, err := rs.readBoardEvents(ctx, boardId)
	if err != nil {
		return err
	}
	return rs.foldInto(ctx, events, boardId, leaderboardKey)
}

func (rs *redisStorage) requireBoard(ctx context.Context, boardId board.ID) error {
	exists, err := rs.client.Exists(ctx, boardHashKey(boardId)).Result()
	if err != nil {
		return fmt.Errorf("storage projection admin: check board '%s': %w", boardId, err)
	}
	if exists == 0 {
		return ErrBoardNotFound
	}
	return nil
}

// Reads the whole ledger, oldest first, retaining only one board's events.
// MVP: one XRANGE reads the whole stream; pagination with COUNT batches is deferred.
func (rs *redisStorage) readBoardEvents(
	ctx context.Context, boardId board.ID,
) ([]ledger.Event, error) {
	entries, err := rs.client.XRange(ctx, ledgerKey, "-", "+").Result()
	if err != nil {
		return nil, fmt.Errorf("storage rebuild: read ledger: %w", err)
	}
	events := make([]ledger.Event, 0)
	for _, entry := range entries {
		if getStreamEntryValue(entry, entryFieldBoardID) != string(boardId) {
			continue
		}
		event, err := projectionEvent(entry)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func projectionEvent(entry redis.XMessage) (ledger.Event, error) {
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

// Drops keyFor(board), then folds that board's events forward:
// `set` assigns the absolute score, `increment` adds the delta.
func (rs *redisStorage) foldInto(
	ctx context.Context, events []ledger.Event, boardId board.ID, keyFor func(board.ID) string,
) error {
	if err := rs.client.Del(ctx, keyFor(boardId)).Err(); err != nil {
		return fmt.Errorf("storage rebuild: drop %q: %w", keyFor(boardId), err)
	}

	for _, event := range events {
		destKey := keyFor(boardId)
		var err error
		switch event.Type {
		case ledger.EventSet:
			err = rs.client.ZAdd(ctx, destKey, redis.Z{Score: event.Amount, Member: event.PlayerID}).Err()
		case ledger.EventIncrement:
			err = rs.client.ZIncrBy(ctx, destKey, event.Amount, event.PlayerID).Err()
		default:
			return fmt.Errorf(
				"%w: ledger event '%s' has unknown type %q",
				ErrInconsistent, event.ID, event.Type,
			)
		}
		if err != nil {
			return fmt.Errorf("storage rebuild: apply event %s: %w", event.ID, err)
		}
	}
	return nil
}

// VerifyProjection replays one board's events into a scratch key and diffs it against
// that board's live projection. Empty result means no drift.
func (rs *redisStorage) VerifyProjection(
	ctx context.Context, boardId board.ID,
) ([]ScoreMismatch, error) {
	if err := rs.requireBoard(ctx, boardId); err != nil {
		return nil, err
	}
	events, err := rs.readBoardEvents(ctx, boardId)
	if err != nil {
		return nil, err
	}

	if err := rs.foldInto(ctx, events, boardId, boardVerifyKey); err != nil {
		return nil, err
	}
	// best-effort cleanup with a fresh context so a cancelled ctx doesn't leak the scratch key
	defer rs.client.Del(context.Background(), boardVerifyKey(boardId))

	return rs.diffBoard(ctx, boardId)
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
