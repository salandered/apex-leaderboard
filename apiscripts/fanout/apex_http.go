package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

type playerStanding struct {
	PlayerID string  `json:"player_id"`
	Score    float64 `json:"score"`
	Rank     int64   `json:"rank"`
}

type listScoresResp struct {
	Scores []playerStanding `json:"scores"`
	Total  int              `json:"total"`
}

type verifyResp struct {
	Mismatches []any `json:"mismatches"`
}

func createBoard(rc *resty.Client) string {
	boardID := apexhttp.SeedBoardID("fanout")
	if err := apexhttp.CreateBoard(rc, boardID, "Fan-out Test"); err != nil {
		log.Fatalf("create board: %v", err)
	}
	return boardID
}

// createPlayers creates n players, giving player i the distinct score i+1.
func createPlayers(rc *resty.Client, n int) []player {
	players := make([]player, 0, n)
	for i := 0; i < n; i++ {
		id, err := apexhttp.CreatePlayer(rc, fmt.Sprintf("fanout-player-%d", i))
		if err != nil {
			log.Fatalf("create player %d: %v", i, err)
		}
		players = append(players, player{id: id, score: float64(i + 1)})
	}
	return players
}

// fetchAllRows pages through the whole leaderboard (limit caps at 100) and returns
// every row plus the reported total.
func fetchAllRows(rc *resty.Client, boardID string) ([]playerStanding, int) {
	const pageSize = 100
	var rows []playerStanding
	total := 0
	for offset := 0; ; offset += pageSize {
		page, err := apexhttp.DoJSON[listScoresResp](rc, resty.MethodGet, fmt.Sprintf(
			"/api/v1/boards/%s/scores?limit=%d&offset=%d", boardID, pageSize, offset,
		), nil, http.StatusOK)
		if err != nil {
			log.Fatalf("list scores: %v", err)
		}
		total = page.Total
		rows = append(rows, page.Scores...)
		if len(page.Scores) == 0 || len(rows) >= total {
			break
		}
	}
	return rows, total
}

func fetchStanding(rc *resty.Client, boardID, playerID string) apexhttp.Standing {
	standing, err := apexhttp.FetchStanding(rc, boardID, playerID)
	if err != nil {
		log.Fatalf("standing %s: %v", playerID, err)
	}
	return standing
}

func verifyProjection(rc *resty.Client, boardID string) {
	resp, err := apexhttp.DoJSON[verifyResp](
		rc, resty.MethodGet, "/api/v1/admin/boards/"+boardID+"/projection/verify", nil, http.StatusOK,
	)
	if err != nil {
		log.Fatalf("projection verify: %v", err)
	}
	if len(resp.Mismatches) != 0 {
		log.Fatalf("projection drift: %d mismatches", len(resp.Mismatches))
	}
}
