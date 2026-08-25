package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/requestid"
	"github.com/salandered/apex/storage"
	"github.com/salandered/httputils/httputils"
)

const (
	playerIDPathValue string = "player_id"
	boardIDPathValue  string = "board_id"

	// pagination query params
	limitQuery  string = "limit"
	offsetQuery string = "offset"

	defaultHistoryLimit int64 = 50  // history page size
	maxHistoryLimit     int64 = 100 // cap on a single history page
	defaultListLimit    int64 = 10  // leaderboard page size (top 10)
	maxListLimit        int64 = 100 // cap on a single leaderboard page
)

// version is overridden at build time via -ldflags "-X ...handlers.version=...".
// Defaults to "dev" for plain `go run`/`go build`.
var version = "dev"

func GetVersion() string {
	return version
}

func HandleRoot(w http.ResponseWriter, req *http.Request) {
	if _, err := fmt.Fprintf(w, "apex version %v", GetVersion()); err != nil {
		slog.ErrorContext(req.Context(), "failed writing root response", "error", err)
	}
}

// requestID uses the middleware's correlation id, falling back to a fresh one when a handler
// runs without the middleware (direct/test calls).
func requestID(req *http.Request) string {
	if id := requestid.FromContext(req.Context()); id != "" {
		return id
	}
	return requestid.New()
}

// Currently POST bodies are small (like a player name)
const maxRequestBodyBytes int64 = 1 << 16 // 64 kb

const idempotencyKeyHeader = "Idempotency-Key"
const maxIdempotencyKeyLen = 128

// Absent -> "".
// Empty or too big -> error.
func readIdempotencyKey(req *http.Request) (string, error) {
	if _, ok := req.Header[idempotencyKeyHeader]; !ok {
		return "", nil
	}
	key := req.Header.Get(idempotencyKeyHeader)
	if key == "" {
		// TODO: consider that empty and no key is the same
		return "", fmt.Errorf("%s must not be empty", idempotencyKeyHeader)
	}
	if len(key) > maxIdempotencyKeyLen {
		return "", fmt.Errorf("%s must be at most %d characters", idempotencyKeyHeader, maxIdempotencyKeyLen)
	}
	return key, nil
}

// want YYYY-MM-DD; returns the UTC start of that day
func parseDateQuery(req *http.Request, name string) (time.Time, error) {
	raw := req.URL.Query().Get(name)
	date, err := apextime.ParseDate(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"invalid query param, want YYYY-MM-DD; param '%v', value '%v'", name, raw)
	}
	return date, nil
}

func boardIdFromPath(req *http.Request) (board.ID, error) {
	boardId := board.ID(req.PathValue(boardIDPathValue))
	if err := boardId.Validate(); err != nil {
		return "", err
	}
	return boardId, nil
}

func playerIdFromPath(req *http.Request) (player.ID, error) {
	playerId := player.ID(req.PathValue(playerIDPathValue))
	if err := playerId.Validate(); err != nil {
		return "", err
	}
	return playerId, nil
}

// Response metadata, one type per paging style.
// TODO: consitent paging style

type offsetMeta struct {
	Limit  int64 `json:"limit"`
	Offset int64 `json:"offset"`
	Total  int64 `json:"total"`
}

type cursorMeta struct {
	Limit     int64  `json:"limit"`
	NextAfter string `json:"next_after"`
}

type limitMeta struct {
	Limit int64 `json:"limit"`
}

type totalMeta struct {
	Total int64 `json:"total"`
}

// Response/Request Utils

func writeRequestError(ctx context.Context, w http.ResponseWriter, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		httputils.WriteError(
			ctx, w, errors.New("request body too large"), http.StatusRequestEntityTooLarge)
		return
	}
	httputils.WriteError(ctx, w, err, http.StatusBadRequest)
}

// maps a storage-layer error to an HTTP response
func writeStorageError(ctx context.Context, w http.ResponseWriter, err error) {
	// client messages duplicate 'err' content, but we might want to hide some internal info
	switch {
	case errors.Is(err, storage.ErrNotFound):
		httputils.WriteError(ctx, w, errors.New("not found"), http.StatusNotFound)
	case errors.Is(err, storage.ErrBoardNotFound):
		httputils.WriteError(ctx, w, errors.New("board not found"), http.StatusNotFound)
	case errors.Is(err, storage.ErrBoardExists):
		httputils.WriteError(ctx, w, errors.New("board already exists"), http.StatusConflict)
	case errors.Is(err, storage.ErrBoardClosed):
		httputils.WriteError(ctx, w, errors.New("board closed"), http.StatusConflict)
	case errors.Is(err, storage.ErrIdempotencyConflict):
		httputils.WriteError(ctx, w, errors.New(
			"idempotency key reused with a different request"), http.StatusConflict)
	case errors.Is(err, storage.ErrScoreOutOfRange):
		httputils.WriteError(ctx, w, errors.New(
			"resulting score must be in [-1e13, 1e13]"), http.StatusConflict)
	default:
		httputils.WriteError(ctx, w, err, http.StatusInternalServerError)
	}
}
