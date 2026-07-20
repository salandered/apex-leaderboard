//go:build integration

package storage

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
)

func (s *StorageSuite) TestCreateBoard() {
	ctx := s.ctx()

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		State:     board.BoardActive,
		CreatedAt: mockedTime,
	})

	// then
	s.Require().NoError(err)

	s.requireEqualBoardHash("summer-contest", "Summer Contest", mockedTimeStr, board.BoardActive)
	s.requireEqualBoardRegistry([]string{"summer-contest"})
}

func (s *StorageSuite) TestCreateBoardIdConflict() {
	ctx := s.ctx()

	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		State:     board.BoardActive,
		CreatedAt: mockedTime,
	}))

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest", // same id
		BoardName: "Impostor",
		State:     board.BoardActive,
		CreatedAt: mockedTime,
	})

	// then
	s.Require().ErrorIs(err, ErrBoardExists)
	s.requireEqualBoardHash("summer-contest", "Summer Contest", mockedTimeStr, board.BoardActive)
	s.requireEqualBoardRegistry([]string{"summer-contest"})
}

func (s *StorageSuite) TestCreateBoardInvalidState() {
	ctx := s.ctx()

	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		State:     board.BoardState("unknown"),
		CreatedAt: mockedTime,
	})

	// then
	s.Require().ErrorIs(err, StorageError)
}

func (s *StorageSuite) TestGetBoard() {
	ctx := s.ctx()

	s.createBoard("summer-contest", "Summer Contest", mockedTime)

	// when
	b, err := s.storage.GetBoard(ctx, "summer-contest")

	// then
	s.Require().NoError(err)
	s.Require().Equal(board.ID("summer-contest"), b.BoardId)
	s.Require().Equal("Summer Contest", b.BoardName)
	s.Require().Equal(mockedTime, b.CreatedAt)
}

func (s *StorageSuite) TestGetBoardMissingReturnsNotFound() {
	_, err := s.storage.GetBoard(s.ctx(), "no-such-board")
	s.Require().ErrorIs(err, ErrBoardNotFound)
}

func (s *StorageSuite) TestListBoardsOrderedByCreatedAt() {
	s.createBoard("b-board", "B", mockedTime)
	s.createBoard("a-board", "A", mockedTime.Add(time.Second))

	// when
	boards, err := s.storage.ListBoards(s.ctx())

	// then
	s.Require().NoError(err)
	s.Require().Len(boards, 2)
	s.Require().Equal(board.ID("b-board"), boards[0].BoardId) // created first
	s.Require().Equal(board.ID("a-board"), boards[1].BoardId)
}

func (s *StorageSuite) TestSetBoardStateIdempotent() {
	s.createBoard("weekly", "W", mockedTime)

	// when
	s.Require().NoError(s.storage.SetBoardState(s.ctx(), "weekly", board.BoardClosed))

	// then
	state, err := s.rawClient.HGet(s.ctx(), boardHashKey("weekly"), boardStateField).Result()
	s.Require().NoError(err)
	s.Require().Equal(string(board.BoardClosed), state)

	// and when
	s.Require().NoError(s.storage.SetBoardState(s.ctx(), "weekly", board.BoardClosed))

	// then (same)
	state, err = s.rawClient.HGet(s.ctx(), boardHashKey("weekly"), boardStateField).Result()
	s.Require().NoError(err)
	s.Require().Equal(string(board.BoardClosed), state)
}

func (s *StorageSuite) TestSetBoardStateOpenClose() {
	s.createBoard("weekly", "W", mockedTime)

	// when
	s.Require().NoError(s.storage.SetBoardState(s.ctx(), "weekly", board.BoardClosed))

	// then
	state, err := s.rawClient.HGet(s.ctx(), boardHashKey("weekly"), boardStateField).Result()
	s.Require().NoError(err)
	s.Require().Equal(string(board.BoardClosed), state)

	// and when
	s.Require().NoError(s.storage.SetBoardState(s.ctx(), "weekly", board.BoardActive))

	// then
	state, err = s.rawClient.HGet(s.ctx(), boardHashKey("weekly"), boardStateField).Result()
	s.Require().NoError(err)
	s.Require().Equal(string(board.BoardActive), state)
}

func (s *StorageSuite) TestSetBoardStateUnknownBoard() {
	err := s.storage.SetBoardState(s.ctx(), "ghost", board.BoardClosed)
	s.Require().ErrorIs(err, ErrBoardNotFound)
}

func (s *StorageSuite) TestSetScoreOnClosedBoardRejected() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "Weekly", mockedTime)
	s.closeBoard("weekly")

	err := s.storage.SetScore(ctx, playerId, "weekly", 10, "r1", "")

	s.Require().ErrorIs(err, ErrBoardClosed)
	s.requireStreamLen(ctx, 0) // rejected writes append nothing
	_, err = s.rawClient.ZScore(ctx, leaderboardKey("weekly"), string(playerId)).Result()
	s.Require().ErrorIs(err, redis.Nil) // projection untouched
}

func (s *StorageSuite) TestIncrementScoreOnClosedBoardRejected() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "Weekly", mockedTime)
	s.closeBoard("weekly")

	err := s.storage.IncrementScore(ctx, playerId, "weekly", 5, "r1", "")

	s.Require().ErrorIs(err, ErrBoardClosed)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestOpenedBoardAcceptsWrites() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "Weekly", mockedTime)
	s.closeBoard("weekly")

	s.Require().NoError(s.storage.SetBoardState(ctx, "weekly", board.BoardActive))

	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 10, "r1", ""))
	s.requireStreamLen(ctx, 1)
}

func (s *StorageSuite) TestAppliedWriteRetryOnClosedBoardIsDeduped() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "Weekly", mockedTime)

	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, "weekly", 10, "req-1", "idem-1"))
	s.closeBoard("weekly")

	// the fact was recorded while the board was open: dedupe wins over the closed check
	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, "weekly", 10, "req-2", "idem-1"))
	s.requireStreamLen(ctx, 1)
}

func (s *StorageSuite) TestReadsWorkOnClosedBoard() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "Weekly", mockedTime)
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 10, "r1", ""))

	s.closeBoard("weekly")

	standing, total, err := s.storage.GetStanding(ctx, playerId, "weekly")
	s.Require().NoError(err)
	s.Require().Equal(int64(1), total)
	s.Require().Equal(10.0, standing.Score)

	history, err := s.storage.PlayerHistory(ctx, playerId, "weekly", 0)
	s.Require().NoError(err)
	s.Require().Len(history, 1)
}

func (s *StorageSuite) TestRebuildFoldsClosedBoardEvents() {
	ctx := s.ctx()
	playerId := s.createPlayer("alice")
	s.createBoard("weekly", "W", mockedTime)
	s.Require().NoError(s.storage.SetScore(ctx, playerId, "weekly", 10, "r1", ""))

	s.closeBoard("weekly")
	s.Require().NoError(s.rawClient.Del(ctx, leaderboardKey("weekly")).Err())

	s.Require().NoError(s.storage.RebuildProjection(ctx, "weekly"))

	score, err := s.rawClient.ZScore(ctx, leaderboardKey("weekly"), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(10.0, score)
}
