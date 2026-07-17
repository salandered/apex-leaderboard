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
	err := s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: playerId, PlayerName: "alice"}, 42.5, "r-create")

	// then
	s.Require().NoError(err)

	name, err := s.rawClient.HGet(ctx, playerHashKey(playerId), "player_name").Result()
	s.Require().NoError(err)
	s.Require().Equal("alice", name)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.5, score)

	// the initial score is recorded in the ledger as a `set` event
	s.requireStreamLen(ctx, 1)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(EventSet), last[eventFieldType])
	s.Require().Equal(string(playerId), last[eventFieldPlayerID])
	s.Require().Equal("42.5", last[eventFieldAmount])
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

	playerId := addPlayer(s) // seeded set event -> stream len 1

	// when
	score, err := s.storage.IncrementScore(ctx, playerId, 5.0, "r-inc")

	// then
	s.Require().NoError(err)
	s.Require().Equal(39.0, score) // 34.0 seeded + 5.0

	// the increment is appended to the ledger without dropping the seed event
	s.requireStreamLen(ctx, 2)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(EventIncrement), last[eventFieldType])
	s.Require().Equal("5", last[eventFieldAmount])
}

func (s *StorageSuite) TestIncrementScoreReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	_, err := s.storage.IncrementScore(ctx, player.GenerateID(), 5.0, "r-inc")

	// then
	s.Require().ErrorIs(err, ErrNotFound)

	// a rejected write is not an event (rule 3)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestSetScore() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := addPlayer(s) // seeded with 34.0

	// when
	err := s.storage.SetScore(ctx, playerId, 100.0, "r-set")

	// then
	s.Require().NoError(err)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(100.0, score)

	s.requireStreamLen(ctx, 2) // seed set + this set
	last := s.lastEvent(ctx)
	s.Require().Equal(string(EventSet), last[eventFieldType])
	s.Require().Equal("100", last[eventFieldAmount])
}

func (s *StorageSuite) TestSetScoreReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	err := s.storage.SetScore(ctx, player.GenerateID(), 100.0, "r-set")

	// then
	s.Require().ErrorIs(err, ErrNotFound)
	s.requireStreamLen(ctx, 0) // rejected write is not an event
}

// TestWorkedSequence replays the doc's worked example (§6):
//
//	set(0) +3 +10 -4 set(50) +10 -4  ->  final score 56, 7 ledger events.
func (s *StorageSuite) TestWorkedSequence() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: playerId, PlayerName: "alice"}, 0, "r1"))

	steps := []struct {
		delta  float64
		reqID  string
		expect float64
	}{
		{3, "r2", 3}, {10, "r3", 13}, {-4, "r4", 9},
	}
	for _, st := range steps {
		score, err := s.storage.IncrementScore(ctx, playerId, st.delta, st.reqID)
		s.Require().NoError(err)
		s.Require().Equal(st.expect, score)
	}

	s.Require().NoError(s.storage.SetScore(ctx, playerId, 50, "r5"))

	score, err := s.storage.IncrementScore(ctx, playerId, 10, "r6")
	s.Require().NoError(err)
	s.Require().Equal(60.0, score)
	score, err = s.storage.IncrementScore(ctx, playerId, -4, "r7")
	s.Require().NoError(err)
	s.Require().Equal(56.0, score)

	projected, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, projected)
	s.requireStreamLen(ctx, 7)
}

// TestIncrementIsIdempotent verifies the dedupe branch: replaying a request_id is a no-op
// that returns the original score and appends no new event.
func (s *StorageSuite) TestIncrementIsIdempotent() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := addPlayer(s) // 34.0, stream len 1

	first, err := s.storage.IncrementScore(ctx, playerId, 10, "r-dup")
	s.Require().NoError(err)
	s.Require().Equal(44.0, first)
	s.requireStreamLen(ctx, 2)

	// same request_id again -> no-op
	retry, err := s.storage.IncrementScore(ctx, playerId, 10, "r-dup")
	s.Require().NoError(err)
	s.Require().Equal(44.0, retry) // score unchanged, not 54
	s.requireStreamLen(ctx, 2)     // no new event
}

// TestHistory checks ledger reads: newest-first order, per-player filtering, and limit.
func (s *StorageSuite) TestHistory() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: alice, PlayerName: "alice"}, 0, "a1"))
	_, err := s.storage.IncrementScore(ctx, alice, 3, "a2")
	s.Require().NoError(err)
	_, err = s.storage.IncrementScore(ctx, alice, 10, "a3")
	s.Require().NoError(err)

	// a second player must not leak into alice's history
	bob := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, 5, "b1"))

	// all of alice's events, newest first
	all, err := s.storage.History(ctx, alice, 0)
	s.Require().NoError(err)
	s.Require().Len(all, 3)
	s.Require().Equal(EventIncrement, all[0].Type)
	s.Require().Equal(10.0, all[0].Amount)
	s.Require().Equal("a3", all[0].RequestID)
	s.Require().Equal(EventSet, all[2].Type) // the create event is oldest
	s.Require().Equal(alice.String(), all[0].PlayerID)
	s.Require().False(all[0].At.IsZero()) // timestamp derived from the stream id

	// limit caps the result
	limited, err := s.storage.History(ctx, alice, 2)
	s.Require().NoError(err)
	s.Require().Len(limited, 2)
	s.Require().Equal("a3", limited[0].RequestID)

	// unknown player yields an empty (non-nil) slice
	none, err := s.storage.History(ctx, player.GenerateID(), 0)
	s.Require().NoError(err)
	s.Require().Empty(none)
}

// TestRebuild proves the ledger is the source of truth: wipe the projection, replay,
// and the scores come back identical.
func (s *StorageSuite) TestRebuild() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// alice replays the doc sequence to 56; bob is a second player with a plain score
	alice := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: alice, PlayerName: "alice"}, 0, "a1"))
	for i, delta := range []float64{3, 10, -4} {
		_, err := s.storage.IncrementScore(ctx, alice, delta, "a"+strconv.Itoa(i+2))
		s.Require().NoError(err)
	}
	s.Require().NoError(s.storage.SetScore(ctx, alice, 50, "a5"))
	_, err := s.storage.IncrementScore(ctx, alice, 6, "a6")
	s.Require().NoError(err)

	bob := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayer(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, 42, "b1"))

	// wipe the projection entirely, then rebuild from the stream
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey).Err())
	s.Require().NoError(s.storage.Rebuild(ctx))

	aliceScore, err := s.rawClient.ZScore(ctx, leaderboardKey, string(alice)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, aliceScore)

	bobScore, err := s.rawClient.ZScore(ctx, leaderboardKey, string(bob)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.0, bobScore)
}

// TestVerifyProjection: a clean projection reports no drift; a write that bypasses the
// ledger is detected; the scratch key is cleaned up afterwards.
func (s *StorageSuite) TestVerifyProjection() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := addPlayer(s) // 34.0 via the script
	_, err := s.storage.IncrementScore(ctx, playerId, 6, "v1")
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
	s.Require().Equal(40.0, mismatches[0].RebuiltScore)
}

func addPlayer(s *StorageSuite) player.ID {
	playerId := player.GenerateID()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// rawClient should be used so tests are independent, but fine for now
	err := s.storage.CreatePlayer(
		ctx,
		&player.Profile{PlayerId: playerId, PlayerName: "bob"},
		34.0,
		"r-seed")
	s.Require().NoError(err)
	return playerId
}

// requireStreamLen asserts the ledger holds exactly n events.
func (s *StorageSuite) requireStreamLen(ctx context.Context, n int64) {
	got, err := s.rawClient.XLen(ctx, streamKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(n, got)
}

// lastEvent returns the field/value map of the newest ledger entry.
func (s *StorageSuite) lastEvent(ctx context.Context) map[string]string {
	entries, err := s.rawClient.XRevRangeN(ctx, streamKey, "+", "-", 1).Result()
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	out := make(map[string]string, len(entries[0].Values))
	for k, v := range entries[0].Values {
		out[k] = v.(string)
	}
	return out
}
