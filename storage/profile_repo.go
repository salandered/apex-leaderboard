package storage

import (
	"context"
	"fmt"

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

func (rs *redisStorage) CreatePlayerProfile(
	ctx context.Context,
	profile *player.Profile,
	requestID string,
) error {
	err := rs.client.HSet(
		ctx,
		playerHashKey(profile.PlayerId),
		profileNameField, profile.PlayerName,
		profileCreatedAtField, apextime.Format(profile.CreatedAt),
	).Err()
	if err != nil {
		return fmt.Errorf("storage put data: %w", err)
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
