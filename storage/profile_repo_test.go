//go:build integration

package storage

import (
	"time"

	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestCreatePlayerProfile() {
	ctx := s.ctx()

	playerId := player.GenerateID()

	// when
	id, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId,
		PlayerName: "alice",
		CreatedAt:  mockedTime,
	}, "")

	// then
	s.Require().NoError(err)
	s.Require().Equal(playerId, id)
	s.requireEqualPlayerHash(playerId, "alice", mockedTimeStr)

	// creating a profile is not a score operation: nothing is appended to the ledger
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestCreatePlayerProfileConflict() {
	ctx := s.ctx()

	playerId := player.GenerateID()
	_, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId,
		PlayerName: "alice",
		CreatedAt:  mockedTime,
	}, "")
	s.Require().NoError(err)

	// when: same candidate id (a UUID collision)
	_, err = s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId, // same id
		PlayerName: "impostor",
		CreatedAt:  mockedTime.Add(time.Hour),
	}, "")

	// then
	s.Require().ErrorIs(err, ErrPlayerExists)
	s.requireEqualPlayerHash(playerId, "alice", mockedTimeStr)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestCreatePlayerIsIdempotent() {
	ctx := s.ctx()

	firstId, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId: player.GenerateID(), PlayerName: "alice", CreatedAt: mockedTime,
	}, "r-1")
	s.Require().NoError(err)

	retryId, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId: player.GenerateID(), PlayerName: "alice", CreatedAt: mockedTime, // same name
	}, "r-1")
	s.Require().NoError(err)
	s.Require().Equal(firstId, retryId) // original id, not the retry's candidate
}

func (s *StorageSuite) TestCreatePlayerIdempotencyKeyConflict() {
	ctx := s.ctx()

	_, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId: player.GenerateID(), PlayerName: "alice", CreatedAt: mockedTime,
	}, "r1")
	s.Require().NoError(err)

	_, err = s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId: player.GenerateID(), PlayerName: "bob", CreatedAt: mockedTime, // different name
	}, "r1")
	s.Require().ErrorIs(err, ErrIdempotencyConflict)
}

func (s *StorageSuite) TestCreatePlayerNoKeyLeavesHashEmpty() {
	ctx := s.ctx()

	_, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId: player.GenerateID(), PlayerName: "alice", CreatedAt: mockedTime,
	}, "")
	s.Require().NoError(err)

	hlen, err := s.rawClient.HLen(ctx, playerIdempotencyHashKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(0), hlen)
}

func (s *StorageSuite) TestGetPlayerProfile() {
	playerId := s.createPlayer("bob")

	// when
	profile, err := s.storage.GetPlayerProfile(s.ctx(), playerId)

	// then
	s.Require().NoError(err)
	s.Require().Equal(playerId, profile.PlayerId)
	s.Require().Equal("bob", profile.PlayerName)
	s.Require().Equal(mockedTime, profile.CreatedAt)
}

func (s *StorageSuite) TestGetPlayerProfileMissingReturnsNotFound() {
	_, err := s.storage.GetPlayerProfile(s.ctx(), player.GenerateID())
	s.Require().ErrorIs(err, ErrNotFound)
}
