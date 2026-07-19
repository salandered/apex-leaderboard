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
	profileNameField      = "player_name"
	profileCreatedAtField = "created_at"
)

func playerHashKey(id player.ID) string {
	return "player:" + string(id)
}

//go:embed scripts/create_player.lua
var createPlayerLua string

var createPlayerScript = redis.NewScript(createPlayerLua)

// TODO: requestID is unused until real idempotency lands (client-supplied Idempotency-Key);
// profile creation is not event-sourced, so nothing records it yet.
func (rs *redisStorage) CreatePlayerProfile(
	ctx context.Context,
	profile *player.Profile,
	requestID string,
) error {
	created, err := createPlayerScript.Run(ctx, rs.client,
		[]string{playerHashKey(profile.PlayerId)},
		profile.PlayerName, apextime.Format(profile.CreatedAt),
	).Int()
	if err != nil {
		return fmt.Errorf("storage create player: %w", err)
	}
	if created == 0 {
		return ErrPlayerExists
	}
	return nil
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
