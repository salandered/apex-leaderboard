package main

import (
	"fmt"
	"log"
	"math/rand/v2"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

// Event mix, in percent: the rest of the range are the ordinary small increments.
const (
	bigIncrementPct = 15
	setPct          = 5
)

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

	standing, err := apexhttp.FetchStanding(rc, boardID, playerID)
	if err != nil {
		log.Fatalf("fetch standing: %v", err)
	}
	if standing.Score != expectedScore {
		log.Fatalf("score mismatch: got %d, want %d", standing.Score, expectedScore)
	}

	history, err := apexhttp.FetchHistory(rc, boardID, playerID, len(written))
	if err != nil {
		log.Fatalf("fetch history: %v", err)
	}
	verifyHistory(history, written)

	if err := apexhttp.VerifyProjection(rc, boardID); err != nil {
		log.Fatalf("projection verify: %v", err)
	}

	fmt.Printf(
		"OK: %d events written, score=%d rank=%d, history matches, projection clean\n",
		len(written),
		standing.Score,
		standing.Rank,
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
