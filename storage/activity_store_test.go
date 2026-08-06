//go:build integration

package storage

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestApplyDailyCountsCreatesMissingKey() {
	ctx := s.ctx()
	playerID := string(player.GenerateID())
	day := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)

	err := s.activityStore.ApplyDailyCounts(ctx, []ledger.Event{
		{ID: "1-0", PlayerID: playerID, CreatedAt: day},
		{ID: "2-0", PlayerID: playerID, CreatedAt: day.Add(time.Hour)},
	}, 30*24*time.Hour)

	s.Require().NoError(err)
	score, err := s.rawClient.ZScore(ctx, activityDailyKey("2026-04-01"), playerID).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(2), score)
}

func (s *StorageSuite) TestApplyDailyCountsIncrementsExistingMember() {
	ctx := s.ctx()
	playerID := string(player.GenerateID())
	key := activityDailyKey("2026-04-02")
	s.Require().NoError(s.rawClient.ZAdd(ctx, key, redis.Z{Member: playerID, Score: 2}).Err())

	err := s.activityStore.ApplyDailyCounts(ctx, []ledger.Event{
		{ID: "1-0", PlayerID: playerID, CreatedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}, 30*24*time.Hour)

	s.Require().NoError(err)
	score, err := s.rawClient.ZScore(ctx, key, playerID).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(3), score)
}

func (s *StorageSuite) TestApplyDailyCountsBucketsByUTCDay() {
	ctx := s.ctx()
	alice := string(player.GenerateID())
	bob := string(player.GenerateID())
	day1 := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)

	err := s.activityStore.ApplyDailyCounts(ctx, []ledger.Event{
		{ID: "1-0", PlayerID: alice, CreatedAt: day1},
		{ID: "2-0", PlayerID: bob, CreatedAt: day1.Add(time.Minute)},
		{ID: "3-0", PlayerID: alice, CreatedAt: day2},
	}, 30*24*time.Hour)

	s.Require().NoError(err)
	first, err := s.rawClient.ZRangeWithScores(ctx, activityDailyKey("2026-04-04"), 0, -1).Result()
	s.Require().NoError(err)
	s.Require().ElementsMatch(
		[]redis.Z{{Member: alice, Score: 1}, {Member: bob, Score: 1}}, first,
	)
	second, err := s.rawClient.ZRangeWithScores(ctx, activityDailyKey("2026-04-05"), 0, -1).Result()
	s.Require().NoError(err)
	s.Require().Equal([]redis.Z{{Member: alice, Score: 1}}, second)
}

func (s *StorageSuite) TestApplyDailyCountsSetsTTL() {
	ctx := s.ctx()
	const ttl = 30 * 24 * time.Hour

	err := s.activityStore.ApplyDailyCounts(ctx, []ledger.Event{
		{
			ID:        "1-0",
			PlayerID:  string(player.GenerateID()),
			CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
		},
	}, ttl)

	s.Require().NoError(err)
	actual, err := s.rawClient.TTL(ctx, activityDailyKey("2026-04-03")).Result()
	s.Require().NoError(err)
	s.Require().Greater(actual, ttl-time.Minute)
	s.Require().LessOrEqual(actual, ttl)
}
