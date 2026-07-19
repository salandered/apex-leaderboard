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
	err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId,
		PlayerName: "alice",
		CreatedAt:  mockedTime,
	}, "r-create")

	// then
	s.Require().NoError(err)
	s.requireEqualPlayerHash(playerId, "alice", mockedTimeStr)

	// creating a profile is not a score operation: nothing is appended to the ledger
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestCreatePlayerProfileConflict() {
	ctx := s.ctx()

	playerId := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId,
		PlayerName: "alice",
		CreatedAt:  mockedTime,
	}, "r-create"))

	// when
	err := s.storage.CreatePlayerProfile(ctx, &player.Profile{
		PlayerId:   playerId, // same id
		PlayerName: "impostor",
		CreatedAt:  mockedTime.Add(time.Hour),
	}, "r-create-2")

	// then
	s.Require().ErrorIs(err, ErrPlayerExists)
	s.requireEqualPlayerHash(playerId, "alice", mockedTimeStr)
	s.requireStreamLen(ctx, 0)
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
