package main

import (
	"flag"
	"log"
	"strings"
	"time"
)

type fanoutConfig struct {
	baseURL     string
	playerCount int
	chunkSize   int
	chunkDelay  time.Duration
	historyOut  string
}

func parseFlags() fanoutConfig {
	baseURL := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	playerCount := flag.Int("players", 200, "number of players to fan out onto one board")
	chunkSize := flag.Int("chunk-size", 25, "players enrolled together before waiting chunk-delay")
	chunkDelay := flag.Duration("chunk-delay", 50*time.Millisecond, "delay between launching chunks")
	historyOut := flag.String("history-out", "_tmp_fanout_history.json", "path to save the top player's score history JSON")
	flag.Parse()

	if *playerCount <= 0 {
		log.Fatal("players must be positive")
	}
	if *chunkSize <= 0 {
		log.Fatal("chunk-size must be positive")
	}

	return fanoutConfig{
		baseURL:     strings.TrimRight(*baseURL, "/"),
		playerCount: *playerCount,
		chunkSize:   *chunkSize,
		chunkDelay:  *chunkDelay,
		historyOut:  *historyOut,
	}
}
