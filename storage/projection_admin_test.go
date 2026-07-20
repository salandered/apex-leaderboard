//go:build integration

package storage

import (
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestRebuildProjection() {
	ctx := s.ctx()

	s.createMainBoard()

	// alice replays the doc sequence to 56; bob is a second player with a plain score
	alice := player.GenerateID()
	_, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: alice, PlayerName: "alice"}, "")
	s.Require().NoError(err)
	for i, delta := range []float64{3, 10, -4} {
		s.Require().NoError(s.storage.IncrementScore(
			ctx, alice, board.MainId, delta, "a"+strconv.Itoa(i+2), "",
		))
	}
	s.Require().NoError(s.storage.SetScore(ctx, alice, board.MainId, 50, "a5", ""))
	s.Require().NoError(s.storage.IncrementScore(ctx, alice, board.MainId, 6, "a6", ""))

	bob := player.GenerateID()
	_, err = s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, "")
	s.Require().NoError(err)
	s.Require().NoError(s.storage.SetScore(ctx, bob, board.MainId, 42, "b2", ""))

	// wipe the projection, then rebuild
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey(board.MainId)).Err())
	s.Require().NoError(s.storage.RebuildProjection(ctx, board.MainId))

	aliceScore, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(alice)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, aliceScore)

	bobScore, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(bob)).Result()
	s.Require().NoError(err)
	s.Require().Equal(42.0, bobScore)
}

func (s *StorageSuite) TestRebuildProjectionOnlyAffectsRequestedBoard() {
	ctx := s.ctx()
	s.createMainBoard()
	s.createBoard("weekly", "Weekly", mockedTime)
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 10, "r1", ""))
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 20, "r2", ""))

	s.Require().NoError(
		s.rawClient.ZAdd(ctx, leaderboardKey(board.MainId), redis.Z{Score: 999, Member: string(playerId)}).Err(),
	)
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey("weekly")).Err())

	s.Require().NoError(s.storage.RebuildProjection(ctx, "weekly"))

	mainScore, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(999.0, mainScore)
	weeklyScore, err := s.rawClient.ZScore(ctx, leaderboardKey("weekly"), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(20.0, weeklyScore)
}

func (s *StorageSuite) TestRebuildProjectionDoesNotConsultBoardRegistry() {
	ctx := s.ctx()
	s.createBoard("weekly", "Weekly", mockedTime)
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 20, "r1", ""))
	s.Require().NoError(s.rawClient.ZRem(ctx, boardsRegistryKey, "weekly").Err())
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey("weekly")).Err())

	s.Require().NoError(s.storage.RebuildProjection(ctx, "weekly"))

	score, err := s.rawClient.ZScore(ctx, leaderboardKey("weekly"), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(20.0, score)
}

func (s *StorageSuite) TestRebuildProjectionUnknownBoard() {
	ctx := s.ctx()
	s.Require().NoError(s.rawClient.ZAdd(
		ctx, boardsRegistryKey, redis.Z{Score: 1, Member: "ghost"},
	).Err())

	err := s.storage.RebuildProjection(ctx, "ghost")

	s.Require().ErrorIs(err, ErrBoardNotFound)
}

func (s *StorageSuite) TestRebuildProjectionEmptyBoardDropsUnexpectedProjection() {
	ctx := s.ctx()
	s.createBoard("weekly", "Weekly", mockedTime)
	s.Require().NoError(s.rawClient.ZAdd(
		ctx, leaderboardKey("weekly"), redis.Z{Score: 10, Member: string(player.GenerateID())},
	).Err())

	s.Require().NoError(s.storage.RebuildProjection(ctx, "weekly"))

	exists, err := s.rawClient.Exists(ctx, leaderboardKey("weekly")).Result()
	s.Require().NoError(err)
	s.Require().Zero(exists)
}

func (s *StorageSuite) TestRebuildProjectionRejectsUnknownMatchingEventTypeBeforeDroppingProjection() {
	ctx := s.ctx()
	s.createBoard("weekly", "Weekly", mockedTime)
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.rawClient.ZAdd(
		ctx, leaderboardKey("weekly"), redis.Z{Score: 15, Member: string(playerId)},
	).Err())
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:      "adjust",
			entryFieldPlayerID:  string(playerId),
			entryFieldBoardID:   "weekly",
			entryFieldAmount:    "5",
			entryFieldRequestID: "broken-1",
		},
	}).Err())

	err := s.storage.RebuildProjection(ctx, "weekly")

	s.Require().ErrorIs(err, ErrInconsistent)
	score, scoreErr := s.rawClient.ZScore(ctx, leaderboardKey("weekly"), string(playerId)).Result()
	s.Require().NoError(scoreErr)
	s.Require().Equal(15.0, score)
}

func (s *StorageSuite) TestRebuildProjectionRejectsMalformedMatchingEvent() {
	ctx := s.ctx()
	s.createBoard("weekly", "Weekly", mockedTime)
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:     string(ledger.EventSet),
			entryFieldPlayerID: string(playerId),
			entryFieldBoardID:  "weekly",
			entryFieldAmount:   "not-a-number",
		},
	}).Err())

	err := s.storage.RebuildProjection(ctx, "weekly")

	s.Require().ErrorIs(err, ErrInconsistent)
}

func (s *StorageSuite) TestVerifyProjection() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 34, "v0", ""))
	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, board.MainId, 6, "v1", ""))

	// consistent: every write went through the script
	mismatches, err := s.storage.VerifyProjection(ctx, board.MainId)
	s.Require().NoError(err)
	s.Require().Empty(mismatches)

	exists, err := s.rawClient.Exists(ctx, boardVerifyKey(board.MainId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(0), exists)

	// corrupt the projection directly -> drift is detected
	s.Require().NoError(s.rawClient.ZIncrBy(ctx, leaderboardKey(board.MainId), 1000, string(playerId)).Err())
	mismatches, err = s.storage.VerifyProjection(ctx, board.MainId)
	s.Require().NoError(err)
	s.Require().Len(mismatches, 1)
	s.Require().Equal(string(board.MainId), mismatches[0].BoardID)
	s.Require().Equal(string(playerId), mismatches[0].PlayerID)
	s.Require().Equal(1040.0, mismatches[0].LiveScore) // 34 + 6 + 1000
	s.Require().Equal(40.0, mismatches[0].ReplayScore)
}

func (s *StorageSuite) TestVerifyProjectionOnlyChecksRequestedBoard() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")
	s.createBoard("weekly", "Weekly", mockedTime)
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 10, "v-m", ""))
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 20, "v-w", ""))
	s.Require().NoError(
		s.rawClient.ZIncrBy(ctx, leaderboardKey("weekly"), 5, string(playerId)).Err(),
	)

	mismatches, err := s.storage.VerifyProjection(ctx, board.MainId)

	s.Require().NoError(err)
	s.Require().Empty(mismatches)

	mismatches, err = s.storage.VerifyProjection(ctx, "weekly")
	s.Require().NoError(err)
	s.Require().Len(mismatches, 1)
	s.Require().Equal("weekly", mismatches[0].BoardID)
	s.Require().Equal(25.0, mismatches[0].LiveScore)
	s.Require().Equal(20.0, mismatches[0].ReplayScore)
}

func (s *StorageSuite) TestVerifyProjectionDoesNotConsultBoardRegistry() {
	ctx := s.ctx()
	s.createBoard("weekly", "Weekly", mockedTime)
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 20, "r1", ""))
	s.Require().NoError(s.rawClient.ZRem(ctx, boardsRegistryKey, "weekly").Err())

	mismatches, err := s.storage.VerifyProjection(ctx, "weekly")

	s.Require().NoError(err)
	s.Require().Empty(mismatches)
}

func (s *StorageSuite) TestVerifyProjectionIgnoresMalformedEventsForOtherBoards() {
	ctx := s.ctx()
	s.createMainBoard()
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 20, "r1", ""))
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:     "unknown",
			entryFieldPlayerID: "not-a-player-id",
			entryFieldBoardID:  "weekly",
			entryFieldAmount:   "not-a-number",
		},
	}).Err())

	mismatches, err := s.storage.VerifyProjection(ctx, board.MainId)

	s.Require().NoError(err)
	s.Require().Empty(mismatches)
}

func (s *StorageSuite) TestVerifyProjectionUnknownBoard() {
	mismatches, err := s.storage.VerifyProjection(s.ctx(), "ghost")

	s.Require().ErrorIs(err, ErrBoardNotFound)
	s.Require().Nil(mismatches)
}
