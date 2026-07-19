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
	fmt.Fprintf(w, "apex %s — see /api/v1/scores\n", GetVersion())
}

// TODO: accept a client-supplied key (Idempotency-Key header).
// Generating it server-side makes every request unique
func newRequestID() string {
	return uuid.NewString()
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

// boardIdFromPath reads {board_id}; the legacy alias routes have no board segment,
// which means the main board.
func boardIdFromPath(req *http.Request) (board.ID, error) {
	raw := req.PathValue(boardIDPathValue)
	if raw == "" {
		return board.MainId, nil
	}
	boardId := board.ID(raw)
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
	slog.Error("internal storage error", "error", err)
	writeErrorToResponse(w, fmt.Errorf("internal server error"), http.StatusInternalServerError)
}
