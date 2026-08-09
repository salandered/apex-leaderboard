// Package apexhttp makes requests to the apex API calls using resty client
// and stores some commmon utils.
// Used by load/verification scripts.
package apexhttp

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

func NewClient(baseURL string, maxConns int) *resty.Client {
	transport := &http.Transport{
		MaxIdleConns:        maxConns,
		MaxIdleConnsPerHost: maxConns,
		MaxConnsPerHost:     maxConns,
		IdleConnTimeout:     30 * time.Second,
	}
	return resty.New().
		SetBaseURL(strings.TrimRight(baseURL, "/")).
		SetTransport(transport).
		SetTimeout(30 * time.Second)
}

// DoJSON runs a JSON request, decodes the response into T, and errors unless the status
// is one of expectedStatuses.
func DoJSON[T any](rc *resty.Client, method, path string, body any, expectedStatuses ...int) (T, error) {
	var result T
	req := rc.R().SetResult(&result)
	if body != nil {
		req.SetBody(body)
	}

	resp, err := req.Execute(method, path)
	if err != nil {
		return result, err
	}

	if slices.Contains(expectedStatuses, resp.StatusCode()) {
		return result, nil
	}
	return result, fmt.Errorf("unexpected status %s: %s", resp.Status(), strings.TrimSpace(string(resp.Body())))
}

// Standing is a single player's placement on a board.
type Standing struct {
	PlayerID string `json:"player_id"`
	Rank     int64  `json:"rank"`
	Score    int64  `json:"score"`
}

type createPlayerResp struct {
	Player struct {
		PlayerID string `json:"player_id"`
	} `json:"player"`
}

type ScoreEvent struct {
	EventID   string `json:"event_id"`
	Type      string `json:"type"`
	PlayerID  string `json:"player_id"`
	BoardID   string `json:"board_id"`
	Amount    int64  `json:"amount"`
	RequestID string `json:"request_id"`
	CreatedAt string `json:"created_at"`
}

type verifyProjectionResp struct {
	Mismatches []any `json:"mismatches"`
}

type History struct {
	PlayerID string       `json:"player_id"`
	Events   []ScoreEvent `json:"events"`
}

// ScorePath is the path to a single player's score/standing on a board.
func ScorePath(boardID, playerID string) string {
	return fmt.Sprintf("/api/v1/boards/%s/scores/%s", boardID, playerID)
}

// SeedBoardID builds a run-scoped board id (prefix plus a millisecond timestamp), so
// repeated runs write to a fresh board instead of colliding on a fixed id.
func SeedBoardID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
}

var playerNames = []string{
	"Alice", "Bob", "Carol", "Dave", "Erin", "Frank",
	"Grace", "Heidi", "Ivan", "Judy", "Kimberly", "Lulu",
	"Mallory", "Nicholas", "Olivia", "Pluto", "Queenie",
	"Rex Complex", "Steven Even", "Todd Odd", "Ursa",
	"Vortex", "Wilson", "Xyla", "Youhan", "Zera",
}

// last 5 digits of the start millisecond
var runStamp = time.Now().UnixMilli() % 100_000

// A pool name, the optional tags, and the run stamp.
// Example: "Alice-73412-smth").
func PlayerName(i int, tags ...string) string {
	parts := append([]string{playerNames[i%len(playerNames)]}, tags...)
	name := fmt.Sprintf("%s-%d", strings.Join(parts, "-"), runStamp)
	if wrap := i / len(playerNames); wrap > 0 {
		name = fmt.Sprintf("%s-%d", name, wrap)
	}
	return name
}

func RandomPlayerName(tags ...string) string {
	return PlayerName(rand.IntN(len(playerNames)), tags...)
}

// CreatePlayer creates a player and returns the server-generated id.
func CreatePlayer(rc *resty.Client, name string) (string, error) {
	player, err := DoJSON[createPlayerResp](rc, resty.MethodPost, "/api/v1/players", map[string]any{
		"player_name": name,
	}, http.StatusCreated)
	if err != nil {
		return "", err
	}
	if player.Player.PlayerID == "" {
		return "", fmt.Errorf("create player returned an empty player_id")
	}
	return player.Player.PlayerID, nil
}

// CreateBoard creates a board with the given id and display name.
func CreateBoard(rc *resty.Client, boardID, name string) error {
	_, err := DoJSON[any](rc, resty.MethodPut, "/api/v1/boards/"+boardID, map[string]any{
		"board_name": name,
	}, http.StatusCreated)
	return err
}

// EnsureBoard is CreateBoard that tolerates the board already existing
func EnsureBoard(rc *resty.Client, boardID, name string) error {
	_, err := DoJSON[any](rc, resty.MethodPut, "/api/v1/boards/"+boardID, map[string]any{
		"board_name": name,
	}, http.StatusCreated, http.StatusConflict)
	return err
}

// SetScore sets a player's score on a board (the first write enrolls the player).
func SetScore(rc *resty.Client, boardID, playerID string, score int64) error {
	_, err := DoJSON[any](rc, resty.MethodPut, ScorePath(boardID, playerID), map[string]any{
		"player_score": score,
	}, http.StatusNoContent)
	return err
}

// IncrementScore adds a delta (may be negative) to a player's score on a board.
func IncrementScore(rc *resty.Client, boardID, playerID string, amount int64) error {
	_, err := DoJSON[any](rc, resty.MethodPost, ScorePath(boardID, playerID)+"/increment", map[string]any{
		"amount": amount,
	}, http.StatusNoContent)
	return err
}

// ListScoresResp is one page of a board's leaderboard.
type ListScoresResp struct {
	Scores   []Standing `json:"scores"`
	Metadata struct {
		Limit  int64 `json:"limit"`
		Offset int64 `json:"offset"`
		Total  int64 `json:"total"`
	} `json:"metadata"`
}

// FetchScores reads one page of a board's leaderboard (highest score first).
func FetchScores(rc *resty.Client, boardID string, limit, offset int) (ListScoresResp, error) {
	path := fmt.Sprintf("/api/v1/boards/%s/scores?limit=%d&offset=%d", boardID, limit, offset)
	return DoJSON[ListScoresResp](rc, resty.MethodGet, path, nil, http.StatusOK)
}

// StandingResp is a placement plus the board's player count (the denominator for rank).
type StandingResp struct {
	Standing Standing `json:"standing"`
	Metadata struct {
		Total int64 `json:"total"`
	} `json:"metadata"`
}

// FetchStanding reads a single player's standing on a board.
func FetchStanding(rc *resty.Client, boardID, playerID string) (StandingResp, error) {
	return DoJSON[StandingResp](rc, resty.MethodGet, ScorePath(boardID, playerID), nil, http.StatusOK)
}

// FetchHistory reads a player's score events on a board (newest first).
// A limit of 0 leaves the page size to the server default.
func FetchHistory(rc *resty.Client, boardID, playerID string, limit int) (History, error) {
	path := ScorePath(boardID, playerID) + "/history"
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	return DoJSON[History](rc, resty.MethodGet, path, nil, http.StatusOK)
}

type ActivityEntry struct {
	PlayerID string `json:"player_id"`
	Count    int64  `json:"count"`
}

// ListDailyActivityResp is one day's most active players.
type ListDailyActivityResp struct {
	Date     string          `json:"date"`
	Entries  []ActivityEntry `json:"entries"`
	Metadata struct {
		Limit int64 `json:"limit"`
	} `json:"metadata"`
}

// FetchDailyActivity reads a day's activity counts (most active first).
func FetchDailyActivity(rc *resty.Client, date string, limit int) (ListDailyActivityResp, error) {
	path := fmt.Sprintf("/api/v1/activity/daily?date=%s&limit=%d", date, limit)
	return DoJSON[ListDailyActivityResp](rc, resty.MethodGet, path, nil, http.StatusOK)
}

// VerifyProjection fails the caller when the board's projection has drifted from its ledger.
func VerifyProjection(rc *resty.Client, boardID string) error {
	resp, err := DoJSON[verifyProjectionResp](
		rc, resty.MethodGet, "/api/v1/admin/boards/"+boardID+"/projection/verify", nil, http.StatusOK,
	)
	if err != nil {
		return err
	}
	if len(resp.Mismatches) != 0 {
		return fmt.Errorf("projection drift: %d mismatches", len(resp.Mismatches))
	}
	return nil
}

// PrintErrors prints up to limit errors and summarizes the rest, so a failing run stays readable.
func PrintErrors(errs []error, limit int) {
	for i, err := range errs {
		if i >= limit {
			fmt.Printf("... %d additional errors omitted\n", len(errs)-limit)
			break
		}
		fmt.Printf("error: %v\n", err)
	}
}

// SaveHistoryToFile writes a player's history to path as indented JSON.
func SaveHistoryToFile(history History, path string) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
