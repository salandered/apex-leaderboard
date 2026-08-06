package main

import (
	"flag"
	"log"
	"time"
)

type randomHistoryConfig struct {
	baseURL       string
	boardID       string // empty creates a run-scoped board
	boardName     string
	eventCount    int
	seed          uint64
	progressEvery int
	timeout       time.Duration
}

func parseFlags() randomHistoryConfig {
	baseURL := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	boardID := flag.String("board", "", "board id to write to (created if missing); empty creates a run-scoped one")
	boardName := flag.String("board-name", "Random History", "display name used when the board is created")
	eventCount := flag.Int("events", 500, "number of score events to write (the first one is a set)")
	seed := flag.Uint64("seed", 0, "random seed for the event sequence; 0 picks a time-based one")
	progressEvery := flag.Int("progress-every", 100, "print a progress line every N events (0 disables)")
	timeout := flag.Duration("timeout", 15*time.Second, "maximum time to wait for the player history projection")
	flag.Parse()

	if *eventCount <= 0 {
		log.Fatal("events must be positive")
	}
	if *timeout <= 0 {
		log.Fatal("timeout must be positive")
	}
	if *seed == 0 {
		*seed = uint64(time.Now().UnixNano())
	}

	return randomHistoryConfig{
		baseURL:       *baseURL,
		boardID:       *boardID,
		boardName:     *boardName,
		eventCount:    *eventCount,
		seed:          *seed,
		progressEvery: *progressEvery,
		timeout:       *timeout,
	}
}
