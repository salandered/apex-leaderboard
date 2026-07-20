package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
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

func createLoadTestPlayer(client *resty.Client) (createPlayerResponse, error) {
	player, err := doJSON[createPlayerResponse](client, resty.MethodPost, "/api/v1/players", map[string]any{
		"player_name": "load-test-player",
	}, http.StatusCreated)
	if err != nil {
		return createPlayerResponse{}, err
	}
	if player.PlayerID == "" {
		return createPlayerResponse{}, fmt.Errorf("create player returned an empty player_id")
	}
	return player, nil
}

func createLoadTestBoard(client *resty.Client, boardID string) error {
	_, err := doJSON[any](client, resty.MethodPut, "/api/v1/boards/"+boardID, map[string]any{
		"board_name": "Load Test",
	}, http.StatusCreated)
	return err
}

func initializePlayerScore(client *resty.Client, scorePath string) error {
	_, err := doJSON[any](client, resty.MethodPut, scorePath, map[string]any{
		"player_score": 0,
	}, http.StatusNoContent)
	return err
}

// runIncrementLoad launches requests in chunks with a delay between them instead of releasing
// them all at once, so the burst doesn't overrun the server's listen backlog and get refused
// before it ever reaches the handler.
func runIncrementLoad(client *resty.Client, incrementPath string, config loadTestConfig) loadTestResult {
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
			go sendIncrementRequest(client, incrementPath, config.amount, requestNumber, &succeeded, errCh, &wg)
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
	client *resty.Client,
	incrementPath string,
	amount float64,
	requestNumber int,
	succeeded *int64,
	errCh chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	_, err := doJSON[any](client, resty.MethodPost, incrementPath, map[string]any{
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

func fetchPlayerStanding(client *resty.Client, scorePath string) (standingResponse, error) {
	return doJSON[standingResponse](client, resty.MethodGet, scorePath, nil, http.StatusOK)
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

// doJSON sends a JSON request and decodes a JSON response into T, relying on resty's SetResult
// to unmarshal only on the statuses it considers successful (2xx, skipping 204 bodies).
func doJSON[T any](
	client *resty.Client,
	method string,
	path string,
	payload any,
	expectedStatuses ...int,
) (T, error) {
	var result T
	req := client.R().SetResult(&result)
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
	return result, fmt.Errorf("unexpected status %s: %s", resp.Status(), strings.TrimSpace(string(resp.Body())))
}

func closeEnough(actual, expected float64) bool {
	scale := math.Max(1, math.Max(math.Abs(actual), math.Abs(expected)))
	return math.Abs(actual-expected) <= scale*1e-12
}

func seedBoardID() string {
	return fmt.Sprintf("load-%d", time.Now().UnixMilli())
}

func main() {
	config := parseFlags()
	client := newLoadTestClient(config.baseURL, config.requestCount)
	defer client.GetClient().CloseIdleConnections()

	boardID := seedBoardID()

	player, err := createLoadTestPlayer(client)
	if err != nil {
		log.Fatalf("create player: %v", err)
	}

	if err := createLoadTestBoard(client, boardID); err != nil {
		log.Fatalf("create board: %v", err)
	}

	scorePath := fmt.Sprintf("/api/v1/boards/%s/scores/%s", boardID, player.PlayerID)
	if err := initializePlayerScore(client, scorePath); err != nil {
		log.Fatalf("initialize score: %v", err)
	}

	fmt.Printf(
		"board=%s player=%s requests=%d amount=%g chunkSize=%d chunkDelay=%s\n",
		boardID, player.PlayerID, config.requestCount, config.amount, config.chunkSize, config.chunkDelay,
	)

	result := runIncrementLoad(client, scorePath+"/increment", config)
	printLoadTestSummary(result, config.requestCount)
	if len(result.Errors) != 0 {
		os.Exit(1)
	}

	standing, err := fetchPlayerStanding(client, scorePath)
	if err != nil {
		log.Fatalf("get standing: %v", err)
	}
	verifyFinalScore(standing, config.requestCount, config.amount)
}
