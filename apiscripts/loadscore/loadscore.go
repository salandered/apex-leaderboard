package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

type loadTestResult struct {
	Succeeded int64
	Errors    []error
	Duration  time.Duration
}

// runIncrementLoad launches requests in chunks with a delay
func runIncrementLoad(rc *resty.Client, incrementPath string, cfg loadTestConfig) loadTestResult {
	var succeeded int64
	errCh := make(chan error, cfg.requestCount)
	var wg sync.WaitGroup
	wg.Add(cfg.requestCount)

	startedAt := time.Now()
	requestNumber := 0
	for chunkStart := 0; chunkStart < cfg.requestCount; chunkStart += cfg.chunkSize {
		chunkEnd := min(chunkStart+cfg.chunkSize, cfg.requestCount)
		for i := chunkStart; i < chunkEnd; i++ {
			requestNumber++
			go sendIncrementRequest(rc, incrementPath, cfg.amount, requestNumber, &succeeded, errCh, &wg)
		}
		if chunkEnd < cfg.requestCount {
			time.Sleep(cfg.chunkDelay)
		}
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	return loadTestResult{
		Succeeded: succeeded,
		Errors:    errs,
		Duration:  time.Since(startedAt),
	}
}

func sendIncrementRequest(
	rc *resty.Client,
	incrementPath string,
	amount float64,
	requestNumber int,
	succeeded *int64,
	errCh chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	_, err := apexhttp.DoJSON[any](
		rc,
		resty.MethodPost,
		incrementPath,
		map[string]any{
			"amount": amount,
		},
		http.StatusOK, http.StatusNoContent,
	)
	if err != nil {
		errCh <- fmt.Errorf("request %d: %w", requestNumber, err)
		return
	}
	atomic.AddInt64(succeeded, 1)
}

func printLoadTestSummary(result loadTestResult, requestCount int) {
	failed := requestCount - int(result.Succeeded)
	fmt.Printf("completed in %s: succeeded=%d failed=%d\n", result.Duration, result.Succeeded, failed)

	printed := 0
	for _, err := range result.Errors {
		if printed < 20 {
			fmt.Printf("error: %v\n", err)
		}
		printed++
	}
	if printed > 20 {
		fmt.Printf("... %d additional errors omitted\n", printed-20)
	}
}

func verifyFinalScore(standing apexhttp.Standing, requestCount int, amount float64) {
	expected := float64(requestCount) * amount
	fmt.Printf(
		"standing: score=%g expected=%g rank=%d total=%d\n",
		standing.Score, expected, standing.Rank, standing.Total,
	)
	if !closeEnough(standing.Score, expected) {
		log.Fatalf("score mismatch: got %g, want %g", standing.Score, expected)
	}
}

func closeEnough(actual, expected float64) bool {
	scale := math.Max(1, math.Max(math.Abs(actual), math.Abs(expected)))
	return math.Abs(actual-expected) <= scale*1e-12
}

func main() {
	cfg := parseFlags()
	rc := apexhttp.NewClient(cfg.baseURL, cfg.requestCount)
	defer rc.GetClient().CloseIdleConnections()

	playerId, boardId, scorePath := createApexFixtures(rc)

	fmt.Printf(
		"board=%s player=%s requests=%d amount=%g chunkSize=%d chunkDelay=%s\n",
		boardId, playerId, cfg.requestCount, cfg.amount, cfg.chunkSize, cfg.chunkDelay,
	)

	result := runIncrementLoad(rc, scorePath+"/increment", cfg)

	printLoadTestSummary(result, cfg.requestCount)

	standing, err := apexhttp.FetchStanding(rc, boardId, playerId)
	if err != nil {
		log.Fatalf("get standing: %v", err)
	}

	// Fetch and persist the ledger before verification, so the artifact survives a mismatch.
	history, err := apexhttp.FetchHistory(rc, boardId, playerId)
	if err != nil {
		log.Fatalf("get history: %v", err)
	}
	if err := apexhttp.SaveHistoryToFile(history, cfg.historyOut); err != nil {
		log.Fatalf("save history: %v", err)
	}
	fmt.Printf("saved %d history events to %s\n", len(history.Events), cfg.historyOut)

	verifyFinalScore(standing, cfg.requestCount, cfg.amount)

	if len(result.Errors) != 0 {
		os.Exit(1)
	}
}
