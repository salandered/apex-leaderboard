//go:build integration

package storage

import (
	"context"
	"time"

	"github.com/salandered/apex/board"
)

func (s *StorageSuite) TestCreateBoard() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		CreatedAt: getMockedTime(s),
	}, "cb1")

	// then
	s.Require().NoError(err)

	name, err := s.rawClient.HGet(ctx, boardHashKey("summer-contest"), boardNameField).Result()
	s.Require().NoError(err)
	s.Require().Equal("Summer Contest", name)

	timestamp, err := s.rawClient.HGet(ctx, boardHashKey("summer-contest"), boardCreatedAtField).Result()
	s.Require().NoError(err)
	s.Require().Equal(mockedTime, timestamp)

	// registered alongside the seeded main board
	boardIds, err := s.rawClient.ZRange(ctx, boardsKey, 0, -1).Result()
	s.Require().NoError(err)
	s.Require().ElementsMatch([]string{"main", "summer-contest"}, boardIds)
}

func (s *StorageSuite) TestCreateBoardConflict() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		CreatedAt: getMockedTime(s),
	}, "cb1"))

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest", // same id
		BoardName: "Impostor",
		CreatedAt: getMockedTime(s),
	}, "cb2")

	// then
	s.Require().ErrorIs(err, ErrBoardExists)
	name, err := s.rawClient.HGet(ctx, boardHashKey("summer-contest"), boardNameField).Result()
	s.Require().NoError(err)
	s.Require().Equal("Summer Contest", name)

	// re-seeding main (a restart) conflicts the same way
	err = s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   board.MainId,
		BoardName: "main",
		CreatedAt: getMockedTime(s),
	}, "cb3")
	s.Require().ErrorIs(err, ErrBoardExists)
}

func (s *StorageSuite) TestGetBoard() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.createBoard("summer-contest", "Summer Contest")

	// when
	b, err := s.storage.GetBoard(ctx, "summer-contest")

	// then
	s.Require().NoError(err)
	s.Require().Equal(board.ID("summer-contest"), b.BoardId)
	s.Require().Equal("Summer Contest", b.BoardName)
	s.Require().Equal(getMockedTime(s), b.CreatedAt)
}

func (s *StorageSuite) TestGetBoardMissingReturnsNotFound() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.storage.GetBoard(ctx, "no-such-board")
	s.Require().ErrorIs(err, ErrBoardNotFound)
}

func (s *StorageSuite) TestListBoards() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// created later but lexically smaller ids — order must follow creation time
	t0 := getMockedTime(s)
	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId: "zzz-board", BoardName: "Z", CreatedAt: t0.Add(time.Second),
	}, "lb1"))
	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId: "aaa-board", BoardName: "A", CreatedAt: t0.Add(2 * time.Second),
	}, "lb2"))

	boards, err := s.storage.ListBoards(ctx)
	s.Require().NoError(err)
	s.Require().Len(boards, 3)
	s.Require().Equal(board.MainId, boards[0].BoardId)
	s.Require().Equal(board.ID("zzz-board"), boards[1].BoardId)
	s.Require().Equal(board.ID("aaa-board"), boards[2].BoardId)
	s.Require().Equal("Z", boards[1].BoardName)
}
