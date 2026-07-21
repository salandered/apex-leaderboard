package main

import (
	"log"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

func createApexFixtures(rc *resty.Client) (string, string, string) {
	playerId, err := apexhttp.CreatePlayer(rc, "load-test-player")
	if err != nil {
		log.Fatalf("create player: %v", err)
	}

	boardId := apexhttp.SeedBoardID("load")
	if err := apexhttp.CreateBoard(rc, boardId, "Load Test"); err != nil {
		log.Fatalf("create board: %v", err)
	}

	if err := apexhttp.SetScore(rc, boardId, playerId, 0); err != nil {
		log.Fatalf("initialize score: %v", err)
	}
	return playerId, boardId, apexhttp.ScorePath(boardId, playerId)
}
