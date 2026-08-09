package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/salandered/apex/apextime"
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

// Currently POST bodies are small (like a player name)
const maxRequestBodyBytes = 1 << 16 // 64 kb

// Decodes the incoming req.
// Checks: only one JSON object; bounded; no unknown fields.
func readJSON(w http.ResponseWriter, req *http.Request, dst any) error {
	req.Body = http.MaxBytesReader(w, req.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()

	// Consider adding branches for json errors like json.SyntaxError, json.UnmarshalTypeError, etc
	if err := dec.Decode(dst); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("body must contain a single JSON object")
	}

	slog.DebugContext(req.Context(), "request decoded", "body", dst)
	return nil
}

func writeJSONToResponse(ctx context.Context, w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")

	rawJSON, err := json.Marshal(data)
	if err != nil {
		WriteErrorToResponse(
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
		"bytes", len(rawJSON), "payload", truncatePayload(rawJSON))
}

type errorResponse struct {
	Error string `json:"error"`
}

func WriteErrorToResponse(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	msg := err.Error()
	if statusCode >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "request failed", "status", statusCode, "error", err)
		msg = "internal server error" // client should not see the actual error
	} else {
		slog.WarnContext(ctx, "request rejected", "status", statusCode, "error", err)
	}

	rawJSON, marshalErr := json.Marshal(errorResponse{Error: msg})
	if marshalErr != nil {
		// providing requestID (via ctx)
		slog.ErrorContext(ctx, "failed marshalling error response", "error", marshalErr)
		// all bad, just return a plain text
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// w.Header().Set("X-Content-Type-Options", "nosniff") // consider adding
	w.WriteHeader(statusCode)

	if _, writeErr := w.Write(rawJSON); writeErr != nil {
		slog.ErrorContext(ctx, "failed writing error response", "status", statusCode, "error", writeErr)
	}
}

func writeRequestError(ctx context.Context, w http.ResponseWriter, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		WriteErrorToResponse(
			ctx, w, fmt.Errorf("request body too large"), http.StatusRequestEntityTooLarge)
		return
	}
	WriteErrorToResponse(ctx, w, err, http.StatusBadRequest)
}

// maps a storage-layer error to an HTTP response
func writeStorageError(ctx context.Context, w http.ResponseWriter, err error) {
	// Errorf messages duplicate 'err' content, but we might want to hide some internal info
	if errors.Is(err, storage.ErrNotFound) {
		WriteErrorToResponse(ctx, w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardNotFound) {
		WriteErrorToResponse(ctx, w, fmt.Errorf("board not found"), http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrBoardExists) {
		WriteErrorToResponse(ctx, w, fmt.Errorf("board already exists"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrBoardClosed) {
		WriteErrorToResponse(ctx, w, fmt.Errorf("board closed"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrIdempotencyConflict) {
		WriteErrorToResponse(ctx, w, fmt.Errorf("idempotency key reused with a different request"), http.StatusConflict)
		return
	}
	if errors.Is(err, storage.ErrScoreOutOfRange) {
		WriteErrorToResponse(ctx, w, fmt.Errorf(
			"resulting score must be in [-1e13, 1e13]"), http.StatusConflict)
		return
	}
	WriteErrorToResponse(ctx, w, err, http.StatusInternalServerError)
}

const maxLoggedPayload = 512

func truncatePayload(rawJSON []byte) string {
	if len(rawJSON) <= maxLoggedPayload {
		return string(rawJSON)
	}
	// json.Marshal emits raw UTF-8, so just a cut might split a rune
	return strings.ToValidUTF8(string(rawJSON[:maxLoggedPayload]), "") + "..."
}
