package storage

import (
	"context"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

type PlayerRepo interface {
	CreatePlayerProfile(ctx context.Context, profile *player.Profile, requestID string) error
	GetPlayerProfile(ctx context.Context, playerId player.ID) (*player.Profile, error)
}

type BoardRepo interface {
	CreateBoard(ctx context.Context, board *board.Board, requestID string) error
}

// Score reads and writes exposed through the API.
// Standings and history are both derived views of the ledger, so they belong together here.
type ScoreRepo interface {
	IncrementScore(ctx context.Context, playerId player.ID, boardId board.ID, amount float64, requestID string) (float64, error)
	SetScore(ctx context.Context, playerId player.ID, boardId board.ID, score float64, requestID string) error

	// Returns a player's standing and the total number of ranked players. Rank is 1-based.
	GetStanding(ctx context.Context, playerId player.ID, boardId board.ID) (Standing, int64, error)

	// Returns one page of the leaderboard (highest score first) and the board size.
	ListStandings(ctx context.Context, boardId board.ID, limit, offset int64) ([]Standing, int64, error)

	// Reads the ledger (newest first) for one player. limit <= 0 means no cap.
	PlayerHistory(ctx context.Context, playerId player.ID, boardId board.ID, limit int64) ([]ledger.Event, error)
}

// Ops tooling over ledger + projection
type ProjectionAdmin interface {
	// Drops the projection and replays the whole ledger into it.
	ReplayLedger(ctx context.Context) error

	// Replays the ledger into a scratch key and compares with an actual board.
	// Empty result means projection matches.
	VerifyProjection(ctx context.Context) ([]ScoreMismatch, error)
}

type Storage interface {
	PlayerRepo
	BoardRepo
	ScoreRepo
	ProjectionAdmin
}
