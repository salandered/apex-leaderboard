//go:build integration

package storage

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

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

func (s *StorageSuite) TestHistory() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	aliceId := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: aliceId, PlayerName: "alice"}, "a0"))
	s.Require().NoError(s.storage.SetScore(ctx, aliceId, board.MainId, 5, "a1"))
	_, err := s.storage.IncrementScore(ctx, aliceId, board.MainId, 3, "a2")
	s.Require().NoError(err)
	_, err = s.storage.IncrementScore(ctx, aliceId, board.MainId, 10, "a3")
	s.Require().NoError(err)

	// a second player must not leak into alice's history
	bob := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, "b1"))

	// all of alice's events, newest first
	all, err := s.storage.PlayerHistory(ctx, aliceId, board.MainId, 0)
	s.Require().NoError(err)
	s.Require().Len(all, 3)
	s.Require().Equal(ledger.EventIncrement, all[0].Type)
	s.Require().Equal(10.0, all[0].Amount)
	s.Require().Equal("a3", all[0].RequestID)
	s.Require().Equal(ledger.EventSet, all[2].Type) // the seeding set event is oldest
	s.Require().Equal(aliceId.String(), all[0].PlayerID)
	s.Require().False(all[0].CreatedAt.IsZero()) // timestamp derived from the stream id

	// limit caps the result
	limited, err := s.storage.PlayerHistory(ctx, aliceId, board.MainId, 2)
	s.Require().NoError(err)
	s.Require().Len(limited, 2)
	s.Require().Equal("a3", limited[0].RequestID)

	// unknown player yields an empty (non-nil) slice
	none, err := s.storage.PlayerHistory(ctx, player.GenerateID(), board.MainId, 0)
	s.Require().NoError(err)
	s.Require().Empty(none)
}

func (s *StorageSuite) TestListScores() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// three players with distinct scores
	seeds := []struct {
		name  string
		score float64
	}{{"alice", 30}, {"bob", 20}, {"carol", 10}}
	ids := make(map[string]player.ID, len(seeds))
	for i, sd := range seeds {
		id := player.GenerateID()
		s.Require().NoError(s.storage.CreatePlayerProfile(ctx,
			&player.Profile{PlayerId: id, PlayerName: sd.name}, "ls"+strconv.Itoa(i)))
		s.Require().NoError(s.storage.SetScore(ctx, id, board.MainId, sd.score, "ls-set"+strconv.Itoa(i)))
		ids[sd.name] = id
	}

	// first page: highest first, ranks 1..2; total is the whole board regardless of the page
	page, total, err := s.storage.ListStandings(ctx, board.MainId, 2, 0)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(page, 2)
	s.Require().Equal(ids["alice"].String(), page[0].PlayerID)
	s.Require().Equal(30.0, page[0].Score)
	s.Require().Equal(int64(1), page[0].Rank)
	s.Require().Equal(ids["bob"].String(), page[1].PlayerID)
	s.Require().Equal(int64(2), page[1].Rank)

	// second page continues the ranking (offset skips the top rows)
	page2, total, err := s.storage.ListStandings(ctx, board.MainId, 2, 2)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(page2, 1)
	s.Require().Equal(ids["carol"].String(), page2[0].PlayerID)
	s.Require().Equal(int64(3), page2[0].Rank)

	// offset past the end -> empty (non-nil) slice, but total still reports the board size
	empty, total, err := s.storage.ListStandings(ctx, board.MainId, 10, 5)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Empty(empty)
}

func (s *StorageSuite) TestGetStanding() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	seeds := []struct {
		name  string
		score float64
	}{{"alice", 30}, {"bob", 20}, {"carol", 10}}
	ids := make(map[string]player.ID, len(seeds))
	for i, sd := range seeds {
		id := player.GenerateID()
		s.Require().NoError(s.storage.CreatePlayerProfile(ctx,
			&player.Profile{PlayerId: id, PlayerName: sd.name}, "pr"+strconv.Itoa(i)))
		s.Require().NoError(s.storage.SetScore(ctx, id, board.MainId, sd.score, "pr-set"+strconv.Itoa(i)))
		ids[sd.name] = id
	}

	// top player is rank 1
	standing, total, err := s.storage.GetStanding(ctx, ids["alice"], board.MainId)
	s.Require().NoError(err)
	s.Require().Equal(ids["alice"].String(), standing.PlayerID)
	s.Require().Equal(int64(1), standing.Rank)
	s.Require().Equal(30.0, standing.Score)
	s.Require().Equal(int64(3), total)

	// a mid-board player
	standing, _, err = s.storage.GetStanding(ctx, ids["bob"], board.MainId)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), standing.Rank)
	s.Require().Equal(20.0, standing.Score)

	// unranked player -> not found
	_, _, err = s.storage.GetStanding(ctx, player.GenerateID(), board.MainId)
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *StorageSuite) TestRebuild() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// alice replays the doc sequence to 56; bob is a second player with a plain score
	alice := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: alice, PlayerName: "alice"}, "a1"))
	for i, delta := range []float64{3, 10, -4} {
		_, err := s.storage.IncrementScore(ctx, alice, board.MainId, delta, "a"+strconv.Itoa(i+2))
		s.Require().NoError(err)
	}
	s.Require().NoError(s.storage.SetScore(ctx, alice, board.MainId, 50, "a5"))
	_, err := s.storage.IncrementScore(ctx, alice, board.MainId, 6, "a6")
	s.Require().NoError(err)

	bob := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, "b1"))
	s.Require().NoError(s.storage.SetScore(ctx, bob, board.MainId, 42, "b2"))

	// wipe the projection entirely, then rebuild from the stream
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey).Err())
	s.Require().NoError(s.storage.ReplayLedger(ctx))

	aliceScore, err := s.rawClient.ZScore(ctx, leaderboardKey, string(alice)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, aliceScore)

	bobScore, err := s.rawClient.ZScore(ctx, leaderboardKey, string(bob)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.0, bobScore)
}

func (s *StorageSuite) TestVerifyProjection() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 34, "v0"))
	_, err := s.storage.IncrementScore(ctx, playerId, board.MainId, 6, "v1")
	s.Require().NoError(err)

	// consistent: every write went through the script
	mismatches, err := s.storage.VerifyProjection(ctx)
	s.Require().NoError(err)
	s.Require().Empty(mismatches)

	// the scratch key must not linger
	exists, err := s.rawClient.Exists(ctx, verifyKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(0), exists)

	// corrupt the projection directly (bypassing the ledger) -> drift is detected
	s.Require().NoError(s.rawClient.ZIncrBy(ctx, leaderboardKey, 1000, string(playerId)).Err())
	mismatches, err = s.storage.VerifyProjection(ctx)
	s.Require().NoError(err)
	s.Require().Len(mismatches, 1)
	s.Require().Equal(string(playerId), mismatches[0].PlayerID)
	s.Require().Equal(1040.0, mismatches[0].LiveScore) // 34 + 6 + 1000
	s.Require().Equal(40.0, mismatches[0].ReplayScore)
}

func createPlayer(s *StorageSuite, name string) player.ID {
	playerId := player.GenerateID()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.rawClient.HSet(
		ctx,
		playerHashKey(playerId),
		profileNameField, name,
		profileCreatedAtField, mockedTime,
	).Err()
	s.Require().NoError(err)
	return playerId
}

// asserts the ledger holds n events
func (s *StorageSuite) requireStreamLen(ctx context.Context, n int64) {
	actual, err := s.rawClient.XLen(ctx, ledgerKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(n, actual)
}

// lastEvent returns the field/value map of the newest ledger entry.
func (s *StorageSuite) lastEvent(ctx context.Context) map[string]string {
	entries, err := s.rawClient.XRevRangeN(ctx, ledgerKey, "+", "-", 1).Result()
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	out := make(map[string]string, len(entries[0].Values))
	for k, v := range entries[0].Values {
		out[k] = v.(string)
	}
	return out
}
