//go:build integration

package storage

import (
	"time"

	"github.com/salandered/apex/board"
)

func (s *StorageSuite) TestCreateBoard() {
	ctx := s.ctx()

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		CreatedAt: mockedTime,
	}, "cb1")

	// then
	s.Require().NoError(err)

	s.requireEqualBoardHash("summer-contest", "Summer Contest", mockedTimeStr)
	s.requireEqualBoardRegistry([]string{"summer-contest"})
}

func (s *StorageSuite) TestCreateBoardConflict() {
	ctx := s.ctx()

	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest",
		BoardName: "Summer Contest",
		CreatedAt: mockedTime,
	}, "cb1"))

	// when
	err := s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   "summer-contest", // same id
		BoardName: "Impostor",
		CreatedAt: mockedTime,
	}, "cb2")

	// then
	s.Require().ErrorIs(err, ErrBoardExists)
	s.requireEqualBoardHash("summer-contest", "Summer Contest", mockedTimeStr)
	s.requireEqualBoardRegistry([]string{"summer-contest"})
}

func (s *StorageSuite) TestGetBoard() {
	ctx := s.ctx()

	s.createBoard("summer-contest", "Summer Contest", mockedTime, "r1")

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
	s.createBoard("b-board", "B", mockedTime, "lb1")
	s.createBoard("a-board", "A", mockedTime.Add(time.Second), "lb2")

	// when
	boards, err := s.storage.ListBoards(s.ctx())

	// then
	s.Require().NoError(err)
	s.Require().Len(boards, 2)
	s.Require().Equal(board.ID("b-board"), boards[0].BoardId) // created first
	s.Require().Equal(board.ID("a-board"), boards[1].BoardId)
}
