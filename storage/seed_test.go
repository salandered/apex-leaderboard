package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/storage"
)

type boardRepoStub struct {
	createErr error
	created   []board.Board
}

func (b *boardRepoStub) CreateBoard(ctx context.Context, board *board.Board) error {
	b.created = append(b.created, *board)
	return b.createErr
}

func (b *boardRepoStub) GetBoard(context.Context, board.ID) (*board.Board, error) {
	return nil, nil
}

func (b *boardRepoStub) SetBoardState(context.Context, board.ID, board.BoardState) error {
	return nil
}

func (b *boardRepoStub) ListBoards(context.Context) ([]board.Board, error) {
	return nil, nil
}

func TestSeedMainBoardSeedsMissingBoard(t *testing.T) {
	stub := &boardRepoStub{}

	// when
	err := storage.SeedMainBoard(stub)

	// then
	require.NoError(t, err)
	require.Len(t, stub.created, 1)
	require.Equal(t, board.MainId, stub.created[0].BoardId)
	require.Equal(t, "main", stub.created[0].BoardName)
	require.False(t, stub.created[0].CreatedAt.IsZero())
}

func TestSeedMainBoardIgnoresExistingBoard(t *testing.T) {
	stub := &boardRepoStub{createErr: storage.ErrBoardExists}

	// when
	err := storage.SeedMainBoard(stub)

	// then
	require.NoError(t, err)
}

func TestSeedMainBoardPropagatesOtherErrors(t *testing.T) {
	stubErr := errors.New("redis unreachable")
	stub := &boardRepoStub{createErr: stubErr}

	// when
	err := storage.SeedMainBoard(stub)

	// then
	require.ErrorIs(t, err, stubErr)
}
