//go:build integration

package storage

import (
	"context"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/player"
)

const (
	mockedTime = "2026-01-17T12:30:00.000Z"
)

func getMockedTime(s *StorageSuite) time.Time {
	time, err := apextime.ApexParse(mockedTime)
	s.Require().NoError(err)
	return time
}

func (s *StorageSuite) TestCreatePlayer() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := player.GenerateID()

	// when
	err := s.storage.CreatePlayerProfile(
		ctx,
		&player.Profile{
			PlayerId:   playerId,
			PlayerName: "alice",
			CreatedAt:  getMockedTime(s),
		},
		"r-create",
	)

	// then
	s.Require().NoError(err)

	name, err := s.rawClient.HGet(ctx, playerHashKey(playerId), "player_name").Result()
	s.Require().NoError(err)
	s.Require().Equal("alice", name)

	timestamp, err := s.rawClient.HGet(ctx, playerHashKey(playerId), "created_at").Result()
	s.Require().NoError(err)
	s.Require().Equal(mockedTime, timestamp)

	// creating a profile is not a score operation: nothing is appended to the ledger
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestGetPlayer() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob")

	// when
	profile, err := s.storage.GetPlayerProfile(ctx, playerId)

	// then
	s.Require().NoError(err)
	s.Require().Equal(playerId, profile.PlayerId)
	s.Require().Equal("bob", profile.PlayerName)
	s.Require().Equal(getMockedTime(s), profile.CreatedAt)
}

func (s *StorageSuite) TestGetPlayerMissingReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	_, err := s.storage.GetPlayerProfile(ctx, player.GenerateID())

	// then
	s.Require().ErrorIs(err, ErrNotFound)
}
