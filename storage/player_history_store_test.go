//go:build integration

package storage

import (
	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestApplyPlayerHistoryIndexesEntryIdsPerPlayerAndBoard() {
	ctx := s.ctx()
	alice := player.GenerateID()
	bob := player.GenerateID()
	weekly := board.ID("weekly")

	err := s.playerHistoryStore.ApplyPlayerHistory(ctx, []ledger.Event{
		{ID: "1000-0", PlayerID: string(alice), BoardID: string(testBoardId), Amount: 5},
		{ID: "1000-1", PlayerID: string(alice), BoardID: string(testBoardId), Amount: 7},
		{ID: "2000-3", PlayerID: string(bob), BoardID: string(weekly), Amount: 9},
	})

	s.Require().NoError(err)
	// member is the entry id, score packs millis * 4096 + seq
	s.requireEqualHistoryIndex(alice, testBoardId, []redis.Z{
		{Member: "1000-0", Score: 1000*seqScale + 0},
		{Member: "1000-1", Score: 1000*seqScale + 1},
	})
	s.requireEqualHistoryIndex(bob, weekly, []redis.Z{
		{Member: "2000-3", Score: 2000*seqScale + 3},
	})
}

func (s *StorageSuite) TestApplyPlayerHistoryRejectsMalformedEntryIdAndWritesNothing() {
	ctx := s.ctx()
	playerId := player.GenerateID()

	err := s.playerHistoryStore.ApplyPlayerHistory(ctx, []ledger.Event{
		{ID: "1000-0", PlayerID: string(playerId), BoardID: string(testBoardId)},
		{ID: "not-a-stream-id", PlayerID: string(playerId), BoardID: string(testBoardId)},
	})

	s.Require().ErrorIs(err, ErrInconsistent)
	s.requireEqualHistoryIndex(playerId, testBoardId, []redis.Z{})
}

func (s *StorageSuite) TestApplyPlayerHistoryClampsSequenceAboveScale() {
	ctx := s.ctx()
	playerId := player.GenerateID()

	err := s.playerHistoryStore.ApplyPlayerHistory(ctx, []ledger.Event{
		{ID: "1000-5000", PlayerID: string(playerId), BoardID: string(testBoardId)},
	})

	s.Require().NoError(err)
	s.requireEqualHistoryIndex(playerId, testBoardId, []redis.Z{
		{Member: "1000-5000", Score: 1000*seqScale + seqScale - 1},
	})
}

func (s *StorageSuite) requireEqualHistoryIndex(
	playerId player.ID, boardId board.ID, expected []redis.Z,
) {
	indexed, err := s.rawClient.ZRangeWithScores(
		s.ctx(), playerHistoryKey(playerId, boardId), 0, -1,
	).Result()
	s.Require().NoError(err)
	s.Require().Equal(expected, indexed)
}
