//go:build integration

package storage

import (
	"context"
	"time"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestIncrementScoreReturnsScore() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob") // profile only, no score yet

	// when
	score, err := s.storage.IncrementScore(ctx, playerId, board.MainId, 5.0, "r-inc")

	// then
	s.Require().NoError(err)
	s.Require().Equal(5.0, score) // first write auto-enrolls, increment starts from 0

	s.requireStreamLen(ctx, 1)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(ledger.EventIncrement), last[entryFieldType])
	s.Require().Equal("5", last[entryFieldAmount])
}

func (s *StorageSuite) TestIncrementScoreReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	_, err := s.storage.IncrementScore(ctx, player.GenerateID(), board.MainId, 5.0, "r-inc")

	// then
	s.Require().ErrorIs(err, ErrNotFound)

	// a rejected write is not an event (rule 2)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestSetScore() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob") // profile only, no score yet

	// when
	err := s.storage.SetScore(ctx, playerId, board.MainId, 100.0, "r-set")

	// then
	s.Require().NoError(err)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(100.0, score)

	s.requireStreamLen(ctx, 1)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(ledger.EventSet), last[entryFieldType])
	s.Require().Equal("100", last[entryFieldAmount])
}

func (s *StorageSuite) TestSetScoreReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	err := s.storage.SetScore(ctx, player.GenerateID(), board.MainId, 100.0, "r-set")

	// then
	s.Require().ErrorIs(err, ErrNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestIncrementIsIdempotent() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob") // profile only, no score yet

	first, err := s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "r-dup")
	s.Require().NoError(err)
	s.Require().Equal(10.0, first)
	s.requireStreamLen(ctx, 1)

	// same request_id again -> no-op
	retry, err := s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "r-dup")
	s.Require().NoError(err)
	s.Require().Equal(10.0, retry) // score unchanged, not 20
	s.requireStreamLen(ctx, 1)     // no new event
}

func (s *StorageSuite) TestScoreOperationSequence() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob")

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 20, "r1"))

	incrementScore(s, playerId, 1, "r2")
	incrementScore(s, playerId, -6, "r3")

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 50, "r4"))

	incrementScore(s, playerId, 10, "r5")
	incrementScore(s, playerId, -4, "r6")

	projected, err := s.rawClient.ZScore(ctx, leaderboardKey, string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, projected)
	s.requireStreamLen(ctx, 6)
}

// Until board CRUD lands only the main board exists: writes naming any other board
// are rejected without appending an event (rule 2).
func (s *StorageSuite) TestScoreWriteUnknownBoardReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	playerId := createPlayer(s, "bob")

	err := s.storage.SetScore(ctx, playerId, board.ID("other"), 100.0, "r-set")
	s.Require().ErrorIs(err, ErrNotFound)

	_, err = s.storage.IncrementScore(ctx, playerId, board.ID("other"), 5.0, "r-inc")
	s.Require().ErrorIs(err, ErrNotFound)

	s.requireStreamLen(ctx, 0)
}

func incrementScore(s *StorageSuite, playerId player.ID, amount float64, reqID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.storage.IncrementScore(ctx, playerId, board.MainId, amount, reqID)
	s.Require().NoError(err)
}
