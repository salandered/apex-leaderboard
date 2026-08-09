package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

// Event mix, in percent: the rest of the range are the ordinary small increments.
const (
	bigIncrementPct = 15
	setPct          = 5
)

// Mirrors the server's cap on GET .../history?limit. A larger value is rejected with a 400.
const historyPageLimit = 100

const pollInterval = 100 * time.Millisecond

type expectedEvent struct {
	eventType string
	amount    int64
}

func main() {
	cfg := parseFlags()
	rng := rand.New(rand.NewPCG(cfg.seed, cfg.seed))

	rc := apexhttp.NewClient(cfg.baseURL, 4)
	defer rc.GetClient().CloseIdleConnections()

	boardID := cfg.boardID
	if boardID == "" {
		boardID = apexhttp.SeedBoardID("random-history")
	}
	if err := apexhttp.EnsureBoard(rc, boardID, cfg.boardName); err != nil {
		log.Fatalf("create board %q: %v", boardID, err)
	}
	playerName := apexhttp.RandomPlayerName()
	playerID, err := apexhttp.CreatePlayer(rc, playerName)
	if err != nil {
		log.Fatalf("create player %q: %v", playerName, err)
	}

	fmt.Printf("board=%s player=%s events=%d seed=%d\n", boardID, playerID, cfg.eventCount, cfg.seed)

	written, expectedScore := writeRandomEvents(rc, boardID, playerID, cfg.eventCount, cfg.progressEvery, rng)

	resp, err := apexhttp.FetchStanding(rc, boardID, playerID)
	if err != nil {
		log.Fatalf("fetch standing: %v", err)
	}
	standing := resp.Standing
	if standing.Score != expectedScore {
		log.Fatalf("score mismatch: got %d, want %d", standing.Score, expectedScore)
	}

	// The API caps a history page and history takes no offset, so only the newest page can be
	// checked event by event. Everything older is still covered by the score and projection checks.
	verified := min(len(written), historyPageLimit)
	history, err := waitForHistory(rc, boardID, playerID, verified, written[len(written)-1], cfg.timeout)
	if err != nil {
		log.Fatal(err)
	}
	verifyHistory(history, written[len(written)-verified:])

	if err := apexhttp.VerifyProjection(rc, boardID); err != nil {
		log.Fatalf("projection verify: %v", err)
	}

	fmt.Printf(
		"OK: %d events written, score=%d rank=%d, newest %d match the history, projection clean\n",
		len(written),
		standing.Score,
		standing.Rank,
		verified,
	)
}

// writeRandomEvents writes eventCount events (starting with set as a known baseline).
// Returns them oldest first along with the score they add up to.
func writeRandomEvents(
	rc *resty.Client,
	boardID, playerID string,
	eventCount, progressEvery int,
	rng *rand.Rand,
) ([]expectedEvent, int64) {
	written := make([]expectedEvent, 0, eventCount)
	var score int64

	for i := 0; i < eventCount; i++ {
		event := expectedEvent{eventType: "increment"}
		switch {
		case i == 0 || rng.IntN(100) < setPct:
			event = expectedEvent{eventType: "set", amount: rng.Int64N(5000)}
		case rng.IntN(100) < bigIncrementPct:
			event.amount = rng.Int64N(1001) - 500
		default:
			event.amount = rng.Int64N(151) - 50
		}

		var err error
		if event.eventType == "set" {
			err = apexhttp.SetScore(rc, boardID, playerID, event.amount)
			score = event.amount
		} else {
			err = apexhttp.IncrementScore(rc, boardID, playerID, event.amount)
			score += event.amount
		}
		if err != nil {
			log.Fatalf("write event %d (%s %d): %v", i, event.eventType, event.amount, err)
		}

		written = append(written, event)
		if progressEvery > 0 && (i+1)%progressEvery == 0 {
			fmt.Printf("  %d/%d events, score=%d\n", i+1, eventCount, score)
		}
	}
	return written, score
}

// The history view is written by an async consumer, so a page fetched right after the last write
// can be stale. Polls until the newest written event sits at the head of a full page:
// the consumer folds in stream order, so by then everything older is indexed too.
func waitForHistory(
	rc *resty.Client,
	boardID, playerID string,
	want int,
	newest expectedEvent,
	timeout time.Duration,
) (apexhttp.History, error) {
	deadline := time.Now().Add(timeout)
	for {
		history, err := apexhttp.FetchHistory(rc, boardID, playerID, want)
		if err != nil {
			return apexhttp.History{}, fmt.Errorf("fetch history: %w", err)
		}
		if len(history.Events) == want &&
			history.Events[0].Type == newest.eventType &&
			history.Events[0].Amount == newest.amount {
			return history, nil
		}
		if time.Now().After(deadline) {
			return apexhttp.History{}, fmt.Errorf(
				"timed out after %s waiting for the player history projection: got %d of %d events",
				timeout,
				len(history.Events),
				want,
			)
		}
		time.Sleep(pollInterval)
	}
}

// verifyHistory checks the ledger returned by the API against what the script wrote: the events
// come back newest first, so they are compared in reverse.
func verifyHistory(history apexhttp.History, written []expectedEvent) {
	if len(history.Events) != len(written) {
		log.Fatalf("history length mismatch: got %d events, want %d", len(history.Events), len(written))
	}

	for i, want := range written {
		got := history.Events[len(history.Events)-1-i]
		if got.Type != want.eventType || got.Amount != want.amount {
			log.Fatalf(
				"history event %d mismatch: got %s %d, want %s %d",
				i,
				got.Type,
				got.Amount,
				want.eventType,
				want.amount,
			)
		}
	}
}
