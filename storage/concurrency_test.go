//go:build integration

package storage

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

// N concurrent increments apply N ops and append N events.
func (s *StorageSuite) TestConcurrentIncrementScoreApplielAllOnce() {
	ctx := s.ctx()
	s.createMainBoard()
	playerId := s.createPlayer("alice")

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Require().NoError(
				s.storage.IncrementScore(ctx, playerId, board.MainId, 1, "r"+strconv.Itoa(i), ""),
			)
		}(i)
	}
	wg.Wait()

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(n), score)
	s.requireStreamLen(ctx, n)
}

// N concurrent creates: one wins, others result in ErrPlayerExists.
func (s *StorageSuite) TestConcurrentCreatePlayerProfileAppliedOne() {
	ctx := s.ctx()
	playerId := player.GenerateID()

	const n = 50
	var wg sync.WaitGroup
	var wins atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.storage.CreatePlayerProfile(
				ctx, &player.Profile{PlayerId: playerId, PlayerName: "alice"}, "",
			)
			if err == nil {
				wins.Add(1)
				return
			}
			s.Require().ErrorIs(err, ErrPlayerExists)
		}(i)
	}
	wg.Wait()

	s.Require().Equal(int64(1), wins.Load())

	profile, err := s.storage.GetPlayerProfile(ctx, playerId)
	s.Require().NoError(err)
	s.Require().Equal(playerId, profile.PlayerId)
}
