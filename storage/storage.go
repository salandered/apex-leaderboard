package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/player"
)

var (
	StorageError    = errors.New("storage")
	ErrNotFound     = fmt.Errorf("%w.not found", StorageError)
	ErrInconsistent = fmt.Errorf("%w.inconsistent", StorageError)
)

type Storage interface {
	CreatePlayer(ctx context.Context, profile *player.Profile, score float64) error
	GetPlayer(ctx context.Context, playerId player.ID) (*player.Profile, float64, error)
	IncrementScore(ctx context.Context, playerId player.ID, amount float64) (float64, error)
}

const (
	leaderboardKey  = "leaderboard"
	playerNameField = "player_name"
)

const (
	pingTimeout       = 5 * time.Second
	pingRetryInterval = 250 * time.Millisecond
)

type redisStorage struct {
	client *redis.Client
}

// builds the profile Hash key
func playerHashKey(id player.ID) string {
	return "player:" + string(id)
}

// writes the profile (Hash) and score (Sorted Set) atomically via MULTI/EXEC
func (rs *redisStorage) CreatePlayer(ctx context.Context, profile *player.Profile, score float64) error {
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
func (rs *redisStorage) GetPlayer(ctx context.Context, playerId player.ID) (*player.Profile, float64, error) {
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

	profile := &player.Profile{
		PlayerId:   playerId,
		PlayerName: name,
	}
	return profile, score, nil
}

func (rs *redisStorage) IncrementScore(ctx context.Context, playerId player.ID, amount float64) (float64, error) {
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

// go-redis writes its diagnostic logs (e.g. pool dial failures) to stderr via a package-global logger.
// We route them through slog with Debug level, because the app already logs similar errors on its side
func init() {
	redis.SetLogger(redisLogger{})
}

// redisLogger adapts go-redis's internal Logging interface to slog.
type redisLogger struct{}

func (redisLogger) Printf(ctx context.Context, format string, v ...any) {
	slog.DebugContext(ctx, fmt.Sprintf(format, v...))
}

// Probes Redis at startup so an unreachable server is reported early
// instead of on the first request.
// On timeout it warns and returns, leaving go-redis to connect lazily.
func pingWithRetry(client *redis.Client, addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		err := client.Ping(ctx).Err()
		if err == nil {
			slog.Info("redis connected", "addr", addr)
			return
		}
		if ctx.Err() != nil { // prefer a real dial error over the deadline
			if lastErr == nil {
				lastErr = err
			}
			break
		}
		lastErr = err
		slog.Debug("redis not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-time.After(pingRetryInterval):
		case <-ctx.Done():
		}
	}
	slog.Warn("redis unreachable at startup, continuing (will connect on first use)",
		"waited", pingTimeout, "error", lastErr)
}

func NewStorage(url string) (Storage, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("storage: parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	pingWithRetry(client, opts.Addr)
	return &redisStorage{client: client}, nil
}
