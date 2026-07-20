package storage

import (
	"context"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

type PlayerRepo interface {
	// idempotencyKey is the optional client-supplied key (empty string skips the idempotency record)
	CreatePlayerProfile(ctx context.Context, profile *player.Profile, idempotencyKey string) (player.ID, error)
	GetPlayerProfile(ctx context.Context, playerId player.ID) (*player.Profile, error)
}

type BoardRepo interface {
	// Create-or-conflict: an existing board id yields ErrBoardExists (never overwrites).
	CreateBoard(ctx context.Context, board *board.Board) error
	GetBoard(ctx context.Context, boardId board.ID) (*board.Board, error)
	// Idempotent state change.
	SetBoardState(ctx context.Context, boardId board.ID, state board.BoardState) error
	// Boards in creation order.
	ListBoards(ctx context.Context) ([]board.Board, error)
}

// Score reads and writes exposed through the API.
type ScoreRepo interface {
	// requestID is the server-generated id;
	// idempotencyKey is the optional client-supplied key (empty string skips the idempotency record)
	IncrementScore(ctx context.Context, playerId player.ID, boardId board.ID, amount float64, requestID, idempotencyKey string) error
	SetScore(ctx context.Context, playerId player.ID, boardId board.ID, score float64, requestID, idempotencyKey string) error

	// Returns a player's standing and the total number of ranked players. Rank is 1-based.
	GetStanding(ctx context.Context, playerId player.ID, boardId board.ID) (Standing, int64, error)

	// Returns one page of the leaderboard (highest score first) and the board size.
	ListStandings(ctx context.Context, boardId board.ID, limit, offset int64) ([]Standing, int64, error)

	// Reads the ledger (newest first) for one player. limit <= 0 means no cap.
	PlayerHistory(ctx context.Context, playerId player.ID, boardId board.ID, limit int64) ([]ledger.Event, error)
}

type ProjectionAdmin interface {
	// Drops one board's projection and rebuilds it from the ledger.
	RebuildProjection(ctx context.Context, boardId board.ID) error

	// Replays one board's ledger events into a scratch key and compares with its projection.
	// Empty result means projection matches.
	VerifyProjection(ctx context.Context, boardId board.ID) ([]ScoreMismatch, error)
}

type Storage interface {
	PlayerRepo
	BoardRepo
	ScoreRepo
	ProjectionAdmin
}
