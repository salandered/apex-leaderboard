package main

import (
	"flag"
	"log"
	"strings"
	"time"
)

type loadTestConfig struct {
	baseURL      string
	requestCount int
	amount       float64
	chunkSize    int
	chunkDelay   time.Duration
	historyOut   string
}

func parseFlags() loadTestConfig {
	baseURL := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	requestCount := flag.Int("requests", 1000, "number of increment requests to send")
	amount := flag.Float64("amount", 1, "amount added by each request")
	chunkSize := flag.Int("chunk-size", 25, "number of requests launched together before waiting chunk-delay")
	chunkDelay := flag.Duration("chunk-delay", 50*time.Millisecond, "delay between launching request chunks")
	historyOut := flag.String("history-out", "_tmp_history.json", "path to save the player's score history JSON")
	flag.Parse()

	if *requestCount <= 0 {
		log.Fatal("requests must be positive")
	}
	if *chunkSize <= 0 {
		log.Fatal("chunk-size must be positive")
	}

	return loadTestConfig{
		baseURL:      strings.TrimRight(*baseURL, "/"),
		requestCount: *requestCount,
		amount:       *amount,
		chunkSize:    *chunkSize,
		chunkDelay:   *chunkDelay,
		historyOut:   *historyOut,
	}
}
