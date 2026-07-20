package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
)

const (
	playerIDPathValue string = "player_id"
	boardIDPathValue  string = "board_id"

	// pagination query params
	limitQuery  string = "limit"
	offsetQuery string = "offset"

	defaultHistoryLimit int64 = 50  // history page size
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
	fmt.Fprintf(w, "apex version %v", GetVersion())
}

func newRequestID() string {
	return uuid.NewString()
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
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return "", "", err
	}
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return "", "", err
	}
	return playerId, boardId, nil
}

// Response Utils

func writeJSONToResponse(w http.ResponseWriter, statusCode int, data any) {
	createHeaders(w)

	rawJSON, err := json.Marshal(data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode) // before Write

	_, err = w.Write(rawJSON)
	if err != nil {
		// headers with the status code were already sent to the client
		slog.Error("failed writing response body", "status", statusCode, "error", err)
		return
	}
	// TODO: trim payload, may be too big
	slog.Debug("response sent", "payload", string(rawJSON))
}

func createHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
}

func writeErrorToResponse(w http.ResponseWriter, err error, statusCode int) {
	http.Error(w, err.Error(), statusCode)
}

// maps a storage-layer error to an HTTP response
func writeStorageError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		writeErrorToResponse(w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardNotFound) {
		writeErrorToResponse(w, fmt.Errorf("board not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardExists) {
		writeErrorToResponse(w, fmt.Errorf("board already exists"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrBoardClosed) {
		writeErrorToResponse(w, fmt.Errorf("board closed"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrIdempotencyConflict) {
		writeErrorToResponse(w, fmt.Errorf("idempotency key reused with a different request"), http.StatusConflict)
		return
	}
	slog.Error("internal storage error", "error", err)
	writeErrorToResponse(w, fmt.Errorf("internal server error"), http.StatusInternalServerError)
}
