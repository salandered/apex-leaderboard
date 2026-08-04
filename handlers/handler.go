package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/requestid"
	"github.com/salandered/apex/storage"
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

// max <= 0 means no cap
func parseIntQuery(req *http.Request, name string, def, min, max int64) (int64, error) {
	raw := req.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid query param, want an integer; param '%v', value '%v'", name, raw)
	}
	if v < min || (v > max && max > 0) {
		if max > 0 {
			return 0, fmt.Errorf(
				"invalid query param, want an integer in [%v, %v]; param '%v', value '%v'",
				min, max, name, raw,
			)
		}
		return 0, fmt.Errorf(
			"invalid query param, want an integer >= %v; param '%v', value '%v'", min, name, raw)
	}
	return v, nil
}

func boardIdFromPath(req *http.Request) (board.ID, error) {
	boardId := board.ID(req.PathValue(boardIDPathValue))
	if err := boardId.Validate(); err != nil {
		return "", err
	}
	return boardId, nil
}

func parsePlayerBoardPathValues(w http.ResponseWriter, req *http.Request) (player.ID, board.ID, error) {
	playerId := player.ID(req.PathValue(playerIDPathValue))
	if err := playerId.Validate(); err != nil {
		writeErrorToResponse(req.Context(), w, err, http.StatusBadRequest)
		return "", "", err
	}
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeErrorToResponse(req.Context(), w, err, http.StatusBadRequest)
		return "", "", err
	}
	return playerId, boardId, nil
}

// Response Utils

func writeJSONToResponse(ctx context.Context, w http.ResponseWriter, statusCode int, data any) {
	createHeaders(w)

	rawJSON, err := json.Marshal(data)
	if err != nil {
		writeErrorToResponse(
			ctx,
			w,
			fmt.Errorf("marshalling response body: %w", err),
			http.StatusInternalServerError,
		)
		return
	}

	w.WriteHeader(statusCode) // before Write

	_, err = w.Write(rawJSON)
	if err != nil {
		// headers with the status code were already sent to the client
		slog.ErrorContext(ctx, "failed writing response body", "status", statusCode, "error", err)
		return
	}
	slog.DebugContext(ctx, "response sent",
		"bytes", len(rawJSON), "payload", truncatePayload(rawJSON),
	)
}

func createHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
}

func writeErrorToResponse(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	if statusCode >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "request failed", "status", statusCode, "error", err)
		http.Error(w, "internal server error", statusCode)
		return
	}
	slog.WarnContext(ctx, "request rejected", "status", statusCode, "error", err)
	http.Error(w, err.Error(), statusCode)
}

// maps a storage-layer error to an HTTP response
func writeStorageError(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		writeErrorToResponse(ctx, w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardNotFound) {
		writeErrorToResponse(ctx, w, fmt.Errorf("board not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardExists) {
		writeErrorToResponse(ctx, w, fmt.Errorf("board already exists"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrBoardClosed) {
		writeErrorToResponse(ctx, w, fmt.Errorf("board closed"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrIdempotencyConflict) {
		writeErrorToResponse(ctx, w, fmt.Errorf("idempotency key reused with a different request"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrScoreOutOfRange) {
		writeErrorToResponse(ctx, w, fmt.Errorf(
			"resulting score must be in [-1e13, 1e13]"), http.StatusConflict)
		return
	}
	writeErrorToResponse(ctx, w, err, http.StatusInternalServerError)
}

const maxLoggedPayload = 512

func truncatePayload(rawJSON []byte) string {
	if len(rawJSON) <= maxLoggedPayload {
		return string(rawJSON)
	}
	// json.Marshal emits raw UTF-8, so just a cut might split a rune
	return strings.ToValidUTF8(string(rawJSON[:maxLoggedPayload]), "") + "..."
}
