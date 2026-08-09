package main

import (
	"log"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

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
		id, err := apexhttp.CreatePlayer(rc, apexhttp.PlayerName(i))
		if err != nil {
			log.Fatalf("create player %d: %v", i, err)
		}
		players = append(players, player{id: id, score: int64(i + 1)})
	}
	return players
}

// fetchAllRows pages through the whole leaderboard and returns
// every row plus the reported total.
func fetchAllRows(rc *resty.Client, boardID string) ([]apexhttp.Standing, int) {
	const pageSize = 100
	var rows []apexhttp.Standing
	total := 0
	for offset := 0; ; offset += pageSize {
		page, err := apexhttp.FetchScores(rc, boardID, pageSize, offset)
		if err != nil {
			log.Fatalf("list scores: %v", err)
		}
		total = int(page.Metadata.Total)
		rows = append(rows, page.Scores...)
		if len(page.Scores) == 0 || len(rows) >= total {
			break
		}
	}
	return rows, total
}

func fetchStanding(rc *resty.Client, boardID, playerID string) apexhttp.Standing {
	resp, err := apexhttp.FetchStanding(rc, boardID, playerID)
	if err != nil {
		log.Fatalf("standing %s: %v", playerID, err)
	}
	return resp.Standing
}

func verifyProjection(rc *resty.Client, boardID string) {
	if err := apexhttp.VerifyProjection(rc, boardID); err != nil {
		log.Fatalf("projection verify: %v", err)
	}
}
