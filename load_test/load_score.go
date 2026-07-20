package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type loadTestConfig struct {
	baseURL      string
	requestCount int
	amount       float64
	chunkSize    int
	chunkDelay   time.Duration
}

type createPlayerResponse struct {
	PlayerID string `json:"player_id"`
}

type standingResponse struct {
	PlayerID string  `json:"player_id"`
	Rank     int64   `json:"rank"`
	Score    float64 `json:"score"`
	Total    int64   `json:"total"`
}

type loadTestResult struct {
	Succeeded int64
	Errors    []error
	Duration  time.Duration
}

func parseFlags() loadTestConfig {
	baseURL := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	requestCount := flag.Int("requests", 1000, "number of increment requests to send")
	amount := flag.Float64("amount", 1, "amount added by each request")
	chunkSize := flag.Int("chunk-size", 50, "number of requests launched together before waiting chunk-delay")
	chunkDelay := flag.Duration("chunk-delay", 20*time.Millisecond, "delay between launching request chunks")
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
	}
}

func newLoadTestClient(maxConns int) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        maxConns,
		MaxIdleConnsPerHost: maxConns,
		MaxConnsPerHost:     maxConns,
		IdleConnTimeout:     30 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

func createLoadTestPlayer(client *http.Client, baseURL string) (createPlayerResponse, error) {
	body, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/players", map[string]any{
		"player_name": "load-test-player",
	}, http.StatusCreated)
	if err != nil {
		return createPlayerResponse{}, err
	}

	var player createPlayerResponse
	if err := json.Unmarshal(body, &player); err != nil {
		return createPlayerResponse{}, fmt.Errorf("decode create player response: %w", err)
	}
	if player.PlayerID == "" {
		return createPlayerResponse{}, fmt.Errorf("create player returned an empty player_id")
	}
	return player, nil
}

func createLoadTestBoard(client *http.Client, baseURL, boardID string) error {
	_, err := doJSON(client, http.MethodPut, baseURL+"/api/v1/boards/"+boardID, map[string]any{
		"board_name": "Load Test",
	}, http.StatusCreated)
	return err
}

func initializePlayerScore(client *http.Client, scoreURL string) error {
	_, err := doJSON(client, http.MethodPut, scoreURL, map[string]any{
		"player_score": 0,
	}, http.StatusNoContent)
	return err
}

// runIncrementLoad launches requests in chunks with a delay between them instead of releasing
// them all at once, so the burst doesn't overrun the server's listen backlog and get refused
// before it ever reaches the handler.
func runIncrementLoad(client *http.Client, incrementURL string, config loadTestConfig) loadTestResult {
	var succeeded int64
	errCh := make(chan error, config.requestCount)
	var wg sync.WaitGroup
	wg.Add(config.requestCount)

	startedAt := time.Now()
	requestNumber := 0
	for chunkStart := 0; chunkStart < config.requestCount; chunkStart += config.chunkSize {
		chunkEnd := min(chunkStart+config.chunkSize, config.requestCount)
		for i := chunkStart; i < chunkEnd; i++ {
			requestNumber++
			go sendIncrementRequest(client, incrementURL, config.amount, requestNumber, &succeeded, errCh, &wg)
		}
		if chunkEnd < config.requestCount {
			time.Sleep(config.chunkDelay)
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
	client *http.Client,
	incrementURL string,
	amount float64,
	requestNumber int,
	succeeded *int64,
	errCh chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	_, err := doJSON(client, http.MethodPost, incrementURL, map[string]any{
		"amount": amount,
	}, http.StatusOK, http.StatusNoContent)
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

func fetchPlayerStanding(client *http.Client, scoreURL string) (standingResponse, error) {
	body, err := doJSON(client, http.MethodGet, scoreURL, nil, http.StatusOK)
	if err != nil {
		return standingResponse{}, err
	}

	var standing standingResponse
	if err := json.Unmarshal(body, &standing); err != nil {
		return standingResponse{}, fmt.Errorf("decode standing response: %w", err)
	}
	return standing, nil
}

func verifyFinalScore(standing standingResponse, requestCount int, amount float64) {
	expected := float64(requestCount) * amount
	fmt.Printf(
		"standing: score=%g expected=%g rank=%d total=%d\n",
		standing.Score, expected, standing.Rank, standing.Total,
	)
	if !closeEnough(standing.Score, expected) {
		log.Fatalf("score mismatch: got %g, want %g", standing.Score, expected)
	}
}

func doJSON(
	client *http.Client,
	method string,
	url string,
	payload any,
	expectedStatuses ...int,
) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	for _, status := range expectedStatuses {
		if resp.StatusCode == status {
			return responseBody, nil
		}
	}
	return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
}

func closeEnough(actual, expected float64) bool {
	scale := math.Max(1, math.Max(math.Abs(actual), math.Abs(expected)))
	return math.Abs(actual-expected) <= scale*1e-12
}

func main() {
	config := parseFlags()
	client := newLoadTestClient(config.requestCount)
	defer client.Transport.(*http.Transport).CloseIdleConnections()

	boardID := fmt.Sprintf("load-%d", time.Now().UnixMilli())

	player, err := createLoadTestPlayer(client, config.baseURL)
	if err != nil {
		log.Fatalf("create player: %v", err)
	}

	if err := createLoadTestBoard(client, config.baseURL, boardID); err != nil {
		log.Fatalf("create board: %v", err)
	}

	scoreURL := fmt.Sprintf("%s/api/v1/boards/%s/scores/%s", config.baseURL, boardID, player.PlayerID)
	if err := initializePlayerScore(client, scoreURL); err != nil {
		log.Fatalf("initialize score: %v", err)
	}

	fmt.Printf(
		"board=%s player=%s requests=%d amount=%g chunkSize=%d chunkDelay=%s\n",
		boardID, player.PlayerID, config.requestCount, config.amount, config.chunkSize, config.chunkDelay,
	)

	result := runIncrementLoad(client, scoreURL+"/increment", config)
	printLoadTestSummary(result, config.requestCount)
	if len(result.Errors) != 0 {
		os.Exit(1)
	}

	standing, err := fetchPlayerStanding(client, scoreURL)
	if err != nil {
		log.Fatalf("get standing: %v", err)
	}
	verifyFinalScore(standing, config.requestCount, config.amount)
}
