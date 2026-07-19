package storage

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
)

const (
	boardsRegistryKey   = "boards" // ZSET registry: member=board_id, score=created_at unix
	boardNameField      = "board_name"
	boardCreatedAtField = "created_at"
)

func boardHashKey(id board.ID) string {
	return "board:" + string(id)
}

//go:embed scripts/create_board.lua
var createBoardLua string

var createBoardScript = redis.NewScript(createBoardLua)

func (rs *redisStorage) CreateBoard(
	ctx context.Context,
	board *board.Board,
	requestID string,
) error {
	created, err := createBoardScript.Run(ctx, rs.client,
		[]string{boardHashKey(board.BoardId), boardsRegistryKey},
		string(board.BoardId), board.BoardName, apextime.Format(board.CreatedAt), board.CreatedAt.Unix(),
	).Int()
	if err != nil {
		return fmt.Errorf("storage create board: %w", err)
	}
	if created == 0 {
		return ErrBoardExists
	}
	return nil
}

func (rs *redisStorage) GetBoard(ctx context.Context, boardId board.ID) (*board.Board, error) {
	fields, err := rs.client.HGetAll(ctx, boardHashKey(boardId)).Result()
	if err != nil {
		return nil, fmt.Errorf("storage get board: %w", err)
	}
	if len(fields) == 0 {
		return nil, ErrBoardNotFound
	}
	return boardFromFields(boardId, fields)
}

// Boards are few by assumption, all hashes are fetched at once:
// consider adding pagination.
func (rs *redisStorage) ListBoards(ctx context.Context) ([]board.Board, error) {
	boardIds, err := rs.client.ZRange(ctx, boardsRegistryKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("storage list boards: %w", err)
	}

	pipe := rs.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(boardIds))
	for _, boardId := range boardIds {
		cmds = append(cmds, pipe.HGetAll(ctx, boardHashKey(board.ID(boardId))))
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage list boards: %w", err)
	}

	boards := make([]board.Board, 0, len(boardIds))

	// Broken boards are skipped, otherwise they would've broken listing endpoint.
	// Should not happen at all, board writes are atomic.
	for i, cmd := range cmds {
		fields, err := cmd.Result()
		if err != nil {
			return nil, fmt.Errorf("storage list boards: %w", err)
		}

		if len(fields) == 0 {
			slog.WarnContext(ctx, "board is registered but its hash is missing, skipped",
				"board_id", boardIds[i])
			continue
		}
		b, err := boardFromFields(board.ID(boardIds[i]), fields)
		if err != nil {
			if errors.Is(err, ErrInconsistent) {
				slog.WarnContext(ctx, "board hash is malformed, skipped",
					"board_id", boardIds[i], "error", err)
				continue
			}
			return nil, err
		}
		boards = append(boards, *b)
	}
	return boards, nil
}

func boardFromFields(id board.ID, fields map[string]string) (*board.Board, error) {
	name, ok := fields[boardNameField]
	if !ok {
		return nil, fmt.Errorf(
			"%w: board '%s' hash missing field '%s'",
			ErrInconsistent, id, boardNameField)
	}
	rawDate, ok := fields[boardCreatedAtField]
	if !ok {
		return nil, fmt.Errorf(
			"%w: board '%s' hash missing field '%s'",
			ErrInconsistent, id, boardCreatedAtField)
	}
	date, err := apextime.Parse(rawDate)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: board '%s' field '%s': parse %q: %v",
			StorageError, id, boardCreatedAtField, rawDate, err)
	}

	return &board.Board{
		BoardId:   id,
		BoardName: name,
		CreatedAt: date,
	}, nil
}
