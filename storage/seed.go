package storage

import (
	"context"
	"errors"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
)

// SeedMainBoard creates the default board if missing.
func SeedMainBoard(s BoardRepo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.CreateBoard(ctx, &board.Board{
		BoardId:   board.MainId,
		BoardName: "main",
		State:     board.BoardActive,
		CreatedAt: apextime.Now(),
	})
	if errors.Is(err, ErrBoardExists) {
		return nil
	}
	return err
}
