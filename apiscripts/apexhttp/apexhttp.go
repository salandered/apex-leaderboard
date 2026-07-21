// Package apexhttp holds the resty client and the apex API calls shared by the
// load/verification scripts (fixtures, scores, standings, history). Per-script
// scenario logic stays in each script.
package apexhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
		SetBaseURL(baseURL).
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

	for _, status := range expectedStatuses {
		if resp.StatusCode() == status {
			return result, nil
		}
	}
	return result, fmt.Errorf("unexpected status %s: %s", resp.Status(), strings.TrimSpace(string(resp.Body())))
}

// Standing is a single player's placement on a board.
type Standing struct {
	PlayerID string  `json:"player_id"`
	Rank     int64   `json:"rank"`
	Score    float64 `json:"score"`
	Total    int64   `json:"total"`
}

type createPlayerResp struct {
	PlayerID string `json:"player_id"`
}

type ScoreEvent struct {
	EventID   string  `json:"event_id"`
	Type      string  `json:"type"`
	PlayerID  string  `json:"player_id"`
	BoardID   string  `json:"board_id"`
	Amount    float64 `json:"amount"`
	RequestID string  `json:"request_id"`
	CreatedAt string  `json:"created_at"`
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

// CreatePlayer creates a player and returns the server-generated id.
func CreatePlayer(rc *resty.Client, name string) (string, error) {
	player, err := DoJSON[createPlayerResp](rc, resty.MethodPost, "/api/v1/players", map[string]any{
		"player_name": name,
	}, http.StatusCreated)
	if err != nil {
		return "", err
	}
	if player.PlayerID == "" {
		return "", fmt.Errorf("create player returned an empty player_id")
	}
	return player.PlayerID, nil
}

// CreateBoard creates a board with the given id and display name.
func CreateBoard(rc *resty.Client, boardID, name string) error {
	_, err := DoJSON[any](rc, resty.MethodPut, "/api/v1/boards/"+boardID, map[string]any{
		"board_name": name,
	}, http.StatusCreated)
	return err
}

// SetScore sets a player's score on a board (the first write enrolls the player).
func SetScore(rc *resty.Client, boardID, playerID string, score float64) error {
	_, err := DoJSON[any](rc, resty.MethodPut, ScorePath(boardID, playerID), map[string]any{
		"player_score": score,
	}, http.StatusNoContent)
	return err
}

// FetchStanding reads a single player's standing on a board.
func FetchStanding(rc *resty.Client, boardID, playerID string) (Standing, error) {
	return DoJSON[Standing](rc, resty.MethodGet, ScorePath(boardID, playerID), nil, http.StatusOK)
}

// FetchHistory reads a player's score events on a board (newest first).
func FetchHistory(rc *resty.Client, boardID, playerID string) (History, error) {
	return DoJSON[History](rc, resty.MethodGet, ScorePath(boardID, playerID)+"/history", nil, http.StatusOK)
}

// SaveHistoryToFile writes a player's history to path as indented JSON.
func SaveHistoryToFile(history History, path string) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
