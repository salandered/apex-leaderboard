package storage

import (
	"context"
	"errors"
	"log/slog"
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

// keeps seeding until it succeeds or ctx is done
func SeedMainBoardWithRetry(ctx context.Context, s BoardRepo, retryInterval time.Duration) error {
	for attempt := 1; ; attempt++ {
		if err := SeedMainBoard(s); err == nil {
			return nil
		} else {
			slog.Warn("seeding main board failed, retrying", "attempt", attempt, "error", err)
		}
		select {
		case <-time.After(retryInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
