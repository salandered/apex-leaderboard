package loadscore

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type createPlayerResp struct {
	PlayerID string `json:"player_id"`
}

type standingResp struct {
	PlayerID string  `json:"player_id"`
	Rank     int64   `json:"rank"`
	Score    float64 `json:"score"`
	Total    int64   `json:"total"`
}

type historyEvent struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Amount    float64 `json:"amount"`
	RequestID string  `json:"request_id"`
	CreatedAt string  `json:"created_at"`
}

type historyResp struct {
	PlayerID string         `json:"player_id"`
	Events   []historyEvent `json:"events"`
}

func seedBoardID() string {
	return fmt.Sprintf("load-%d", time.Now().UnixMilli())
}

func newLoadTestClient(baseURL string, maxConns int) *resty.Client {
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

func createApexFixtures(rc *resty.Client) (string, string, string) {
	playerId, err := createLoadTestPlayer(rc)
	if err != nil {
		log.Fatalf("create player: %v", err)
	}

	boardId, err := createLoadTestBoard(rc)
	if err != nil {
		log.Fatalf("create board: %v", err)
	}

	scorePath := fmt.Sprintf("/api/v1/boards/%s/scores/%s", boardId, playerId)
	if err := initializePlayerScore(rc, scorePath); err != nil {
		log.Fatalf("initialize score: %v", err)
	}
	return playerId, boardId, scorePath
}

func createLoadTestPlayer(rc *resty.Client) (string, error) {
	player, err := doJSON[createPlayerResp](rc, resty.MethodPost, "/api/v1/players", map[string]any{
		"player_name": "load-test-player",
	}, http.StatusCreated)
	if err != nil {
		return "", err
	}
	if player.PlayerID == "" {
		return "", fmt.Errorf("create player returned an empty player_id")
	}
	return player.PlayerID, nil
}

func createLoadTestBoard(rc *resty.Client) (string, error) {
	boardID := seedBoardID()
	_, err := doJSON[any](rc, resty.MethodPut, "/api/v1/boards/"+boardID, map[string]any{
		"board_name": "Load Test",
	}, http.StatusCreated)
	return boardID, err
}

func initializePlayerScore(rc *resty.Client, scorePath string) error {
	_, err := doJSON[any](rc, resty.MethodPut, scorePath, map[string]any{
		"player_score": 0,
	}, http.StatusNoContent)
	return err
}

func fetchPlayerStanding(rc *resty.Client, scorePath string) (standingResp, error) {
	return doJSON[standingResp](rc, resty.MethodGet, scorePath, nil, http.StatusOK)
}

func fetchPlayerHistory(rc *resty.Client, scorePath string) (historyResp, error) {
	return doJSON[historyResp](rc, resty.MethodGet, scorePath+"/history", nil, http.StatusOK)
}

func doJSON[T any](
	rc *resty.Client,
	method string,
	path string,
	payload any,
	expectedStatuses ...int,
) (T, error) {
	var result T
	req := rc.R().SetResult(&result)
	if payload != nil {
		req.SetBody(payload)
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
	return result, fmt.Errorf(
		"unexpected status %s: %s", resp.Status(), strings.TrimSpace(string(resp.Body())),
	)
}
