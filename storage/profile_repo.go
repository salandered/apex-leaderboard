package storage

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/player"
)

const (
	profileNameField         = "player_name"
	profileCreatedAtField    = "created_at"
	playerIdempotencyHashKey = "player:idempotency" // HASH client key -> "player_id|player_name"
)

func playerHashKey(id player.ID) string {
	return "player:" + string(id)
}

//go:embed scripts/create_player.lua
var createPlayerLua string

var createPlayerScript = redis.NewScript(createPlayerLua)

// create_player.lua result codes. Must match the script.
const (
	createCodeCreated             = 1  // profile written under the candidate id
	createCodeDeduped             = 0  // idempotency key seen before with a matching name
	createCodeExists              = -1 // candidate id already exists (UUID collision)
	createCodeIdempotencyConflict = -4 // key reused with a different payload
)

func (rs *redisStorage) CreatePlayerProfile(
	ctx context.Context,
	profile *player.Profile,
	idempotencyKey string,
) (player.ID, error) {
	result, err := createPlayerScript.Run(ctx, rs.client,
		[]string{playerHashKey(profile.PlayerId), playerIdempotencyHashKey},
		profile.PlayerName, apextime.Format(profile.CreatedAt), string(profile.PlayerId), idempotencyKey,
	).Slice()
	if err != nil {
		return "", fmt.Errorf("storage create player: %w", err)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("storage create player: empty script result")
	}
	code, ok := result[0].(int64)
	if !ok {
		return "", fmt.Errorf("storage create player: non-integer script code %v", result[0])
	}
	switch code {
	case createCodeCreated, createCodeDeduped:
		id, ok := result[1].(string)
		if !ok {
			return "", fmt.Errorf("storage create player: non-string player id %v", result[1])
		}
		return player.ID(id), nil
	case createCodeExists:
		return "", ErrPlayerExists
	case createCodeIdempotencyConflict:
		return "", ErrIdempotencyConflict
	default:
		return "", fmt.Errorf("storage create player: unexpected script code %d", code)
	}
}

func (rs *redisStorage) GetPlayerProfile(ctx context.Context, playerId player.ID) (*player.Profile, error) {
	fields, err := rs.client.HGetAll(ctx, playerHashKey(playerId)).Result()
	if err != nil {
		return nil, fmt.Errorf("storage get profile: %w", err)
	}
	if len(fields) == 0 {
		return nil, ErrNotFound
	}

	name, ok := fields[profileNameField]
	if !ok {
		return nil, fmt.Errorf("%w: player '%s' hash missing field '%s'", ErrInconsistent, playerId, profileNameField)
	}
	rawDate, ok := fields[profileCreatedAtField]
	if !ok {
		return nil, fmt.Errorf("%w: player '%s' hash missing field '%s'", ErrInconsistent, playerId, profileCreatedAtField)
	}
	date, err := apextime.Parse(rawDate)
	if err != nil {
		return nil, fmt.Errorf("%w: player '%s' field '%s': parse %q: %v", StorageError, playerId, profileCreatedAtField, rawDate, err)
	}

	profile := &player.Profile{
		PlayerId:   playerId,
		PlayerName: name,
		CreatedAt:  date,
	}
	return profile, nil
}
