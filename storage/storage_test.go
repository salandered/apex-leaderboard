//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/salandered/apex/player"
)

// should be the same as in deployment
const testRedisImage = "redis:8.8.0-alpine"

type StorageSuite struct {
	suite.Suite
	storage   Storage
	rawClient *redis.Client // for assertions + flushing
}

func TestStorageSuite(t *testing.T) {
	suite.Run(t, new(StorageSuite))
}

// launches Redis container (random host port)
func (s *StorageSuite) SetupSuite() {
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, testRedisImage)
	testcontainers.CleanupContainer(s.T(), ctr) // adds to s.T() Cleanup
	s.Require().NoError(err)

	url, err := ctr.ConnectionString(ctx)
	s.Require().NoError(err)

	s.storage, err = NewStorage(url)
	s.Require().NoError(err)

	opts, err := redis.ParseURL(url)
	s.Require().NoError(err)
	s.rawClient = redis.NewClient(opts)
	s.T().Cleanup(func() { s.rawClient.Close() })
}

// cleans up the db so tests stay order-independent
func (s *StorageSuite) SetupTest() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Require().NoError(s.rawClient.FlushDB(ctx).Err())
}

func (s *StorageSuite) TestCreatePlayer() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := player.GenerateID()

	// when
	err := s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: playerId, PlayerName: "alice"}, 42.5)

	// then
	s.Require().NoError(err)

	name, err := s.rawClient.HGet(ctx, playerHashKey(playerId), "player_name").Result()
	s.Require().NoError(err)
	s.Require().Equal("alice", name)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.5, score)
}

func (s *StorageSuite) TestGetPlayerReturnsProfileAndScore() {
	// TODO: add test cased for ErrInconsistent
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := addPlayer(s)

	// when
	profile, score, err := s.storage.GetPlayer(ctx, playerId)

	// then
	s.Require().NoError(err)
	s.Require().Equal(playerId, profile.PlayerId)
	s.Require().Equal("bob", profile.PlayerName)
	s.Require().Equal(34.0, score)
}

func (s *StorageSuite) TestGetPlayerMissingReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	_, _, err := s.storage.GetPlayer(ctx, player.GenerateID())

	// then
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *StorageSuite) TestIncrementScoreReturnsScore() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := addPlayer(s)

	// when
	score, err := s.storage.IncrementScore(ctx, playerId, 5.0)

	// then
	s.Require().NoError(err)
	s.Require().Equal(39.0, score) // 34.0 seeded + 5.0
}

func (s *StorageSuite) TestIncrementScoreReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	_, err := s.storage.IncrementScore(ctx, player.GenerateID(), 5.0)

	// then
	s.Require().ErrorIs(err, ErrNotFound)
}

func addPlayer(s *StorageSuite) player.ID {
	playerId := player.GenerateID()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// rawClient should be used so tests are independent, but fine for now
	err := s.storage.CreatePlayer(
		ctx,
		&player.Profile{PlayerId: playerId, PlayerName: "bob"},
		34.0)
	s.Require().NoError(err)
	return playerId
}
