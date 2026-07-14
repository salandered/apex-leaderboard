package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/models"
	playerid "github.com/salandered/apex/player_id"
)

var (
	StorageError    = errors.New("storage")
	ErrNotFound     = fmt.Errorf("%w.not found", StorageError)
	ErrInconsistent = fmt.Errorf("%w.inconsistent", StorageError)
)

type Storage interface {
	CreatePlayer(ctx context.Context, profile *models.Profile, score float64) error
	GetPlayer(ctx context.Context, playerId playerid.PlayerId) (*models.Profile, float64, error)
	IncrementScore(ctx context.Context, playerId playerid.PlayerId, amount float64) (float64, error)
}

const (
	leaderboardKey  = "leaderboard"
	playerNameField = "player_name"
)

type redisStorage struct {
	client *redis.Client
}

// builds the profile Hash key
func playerHashKey(id playerid.PlayerId) string {
	return "player:" + string(id)
}

// writes the profile (Hash) and score (Sorted Set) atomically via MULTI/EXEC
func (rs *redisStorage) CreatePlayer(ctx context.Context, profile *models.Profile, score float64) error {
	playerId := string(profile.PlayerId)
	_, err := rs.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, playerHashKey(profile.PlayerId), playerNameField, profile.PlayerName)
		pipe.ZAdd(ctx, leaderboardKey, redis.Z{Score: score, Member: playerId})
		return nil
	})
	if err != nil {
		return fmt.Errorf("storage put data: %w", err)
	}
	return nil
}

// reads the profile (Hash) and score (Sorted Set) for a player
func (rs *redisStorage) GetPlayer(ctx context.Context, playerId playerid.PlayerId) (*models.Profile, float64, error) {
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

	profile := &models.Profile{
		PlayerId:   playerId,
		PlayerName: name,
	}
	return profile, score, nil
}

func (rs *redisStorage) IncrementScore(ctx context.Context, playerId playerid.PlayerId, amount float64) (float64, error) {
	// ZIncrXX removed in v9 https://pkg.go.dev/github.com/go-redis/redis/v8#Client.ZIncrXX
	score, err := rs.client.ZAddArgsIncr(
		ctx,
		leaderboardKey,
		redis.ZAddArgs{
			XX: true, // no increment if no key
			Members: []redis.Z{{
				Score:  amount,
				Member: string(playerId)}}}).Result()

	if errors.Is(err, redis.Nil) {
		return score, ErrNotFound
	}
	if err != nil {
		return score, fmt.Errorf("storage increment score: %w", err)
	}
	return score, nil
}

// Builds a Redis Storage.
// go-redis connects lazily on first use, a problem will be seen during the request.
func NewStorage(url string) (Storage, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("storage: parse redis url: %w", err)
	}
	return &redisStorage{client: redis.NewClient(opts)}, nil
}
