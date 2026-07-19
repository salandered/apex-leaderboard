//go:build integration

package storage

import (
	"strconv"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestRebuild() {
	ctx := s.ctx()

	s.createMainBoard()

	// alice replays the doc sequence to 56; bob is a second player with a plain score
	alice := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: alice, PlayerName: "alice"}, "a1"))
	for i, delta := range []float64{3, 10, -4} {
		_, err := s.storage.IncrementScore(
			ctx, alice, board.MainId, delta, "a"+strconv.Itoa(i+2),
		)
		s.Require().NoError(err)
	}
	s.Require().NoError(s.storage.SetScore(ctx, alice, board.MainId, 50, "a5"))
	_, err := s.storage.IncrementScore(ctx, alice, board.MainId, 6, "a6")
	s.Require().NoError(err)

	bob := player.GenerateID()
	s.Require().NoError(s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, "b1"))
	s.Require().NoError(s.storage.SetScore(ctx, bob, board.MainId, 42, "b2"))

	// wipe the projection, then rebuild
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey(board.MainId)).Err())
	s.Require().NoError(s.storage.ReplayLedger(ctx))

	aliceScore, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(alice)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, aliceScore)

	bobScore, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(bob)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.0, bobScore)
}

func (s *StorageSuite) TestVerifyProjection() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 34, "v0"))
	_, err := s.storage.IncrementScore(ctx, playerId, board.MainId, 6, "v1")
	s.Require().NoError(err)

	// consistent: every write went through the script
	mismatches, err := s.storage.VerifyProjection(ctx)
	s.Require().NoError(err)
	s.Require().Empty(mismatches)

	exists, err := s.rawClient.Exists(ctx, boardVerifyKey(board.MainId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(0), exists)

	// corrupt the projection directly -> drift is detected
	s.Require().NoError(s.rawClient.ZIncrBy(ctx, leaderboardKey(board.MainId), 1000, string(playerId)).Err())
	mismatches, err = s.storage.VerifyProjection(ctx)
	s.Require().NoError(err)
	s.Require().Len(mismatches, 1)
	s.Require().Equal(string(board.MainId), mismatches[0].BoardID)
	s.Require().Equal(string(playerId), mismatches[0].PlayerID)
	s.Require().Equal(1040.0, mismatches[0].LiveScore) // 34 + 6 + 1000
	s.Require().Equal(40.0, mismatches[0].ReplayScore)
}

func (s *StorageSuite) TestVerifyProjectionOneBoardCorruptedOtherOk() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")
	s.createBoard("weekly", "Weekly", mockedTime, "r1")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 10, "v-m"))
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.ID("weekly"), 20, "v-w"))

	// corrupt only the weekly board
	s.Require().NoError(
		s.rawClient.ZIncrBy(ctx, leaderboardKey("weekly"), 5, string(playerId)).Err(),
	)

	// when
	mismatches, err := s.storage.VerifyProjection(ctx)

	// then
	s.Require().NoError(err)
	s.Require().Len(mismatches, 1)
	s.Require().Equal("weekly", mismatches[0].BoardID)
	s.Require().Equal(25.0, mismatches[0].LiveScore)
	s.Require().Equal(20.0, mismatches[0].ReplayScore)
}
