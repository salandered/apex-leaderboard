package storage

import (
	"context"
	"fmt"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
)

const (
	boardNameField      = "board_name"
	boardCreatedAtField = "created_at"
)

// builds the profile Hash key
func boardHashKey(id board.ID) string {
	return "board:" + string(id)
}

// We can't have empty ZSET so we dont created it here // TODO: verify
func (rs *redisStorage) CreateBoard(
	ctx context.Context,
	board *board.Board,
	requestID string,
) error {
	err := rs.client.HSet(
		ctx,
		boardHashKey(board.BoardId),
		boardNameField, board.BoardName,
		boardCreatedAtField, apextime.ApexFormat(board.CreatedAt),
	).Err()
	if err != nil {
		return fmt.Errorf("storage put data: %w", err)
	}
	return nil
}
