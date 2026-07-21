//go:build integration

package storage

import (
	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestListDailyActivityReturnsHighestCountsFirst() {
	ctx := s.ctx()
	alice := string(player.GenerateID())
	bob := string(player.GenerateID())
	key := activityDailyKey("2026-05-01")
	s.Require().NoError(s.rawClient.ZAdd(ctx, key,
		redis.Z{Member: alice, Score: 3},
		redis.Z{Member: bob, Score: 1},
	).Err())

	entries, err := s.storage.ListDailyActivity(ctx, "2026-05-01", 10)

	s.Require().NoError(err)
	s.Require().Equal([]ActivityEntry{
		{PlayerID: alice, Count: 3},
		{PlayerID: bob, Count: 1},
	}, entries)
}

func (s *StorageSuite) TestListDailyActivityHonorsLimit() {
	ctx := s.ctx()
	key := activityDailyKey("2026-05-02")
	s.Require().NoError(s.rawClient.ZAdd(ctx, key,
		redis.Z{Member: string(player.GenerateID()), Score: 3},
		redis.Z{Member: string(player.GenerateID()), Score: 2},
		redis.Z{Member: string(player.GenerateID()), Score: 1},
	).Err())

	entries, err := s.storage.ListDailyActivity(ctx, "2026-05-02", 2)

	s.Require().NoError(err)
	s.Require().Len(entries, 2)
}

func (s *StorageSuite) TestListDailyActivityUnknownDate() {
	entries, err := s.storage.ListDailyActivity(s.ctx(), "2026-05-03", 10)

	s.Require().NoError(err)
	s.Require().Empty(entries)
}
