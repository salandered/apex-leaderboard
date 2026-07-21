package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

type player struct {
	id    string
	score float64
}

// rankingsShown caps how many leaderboard rows a run prints.
const rankingsShown = 10

func main() {
	cfg := parseFlags()
	rc := apexhttp.NewClient(cfg.baseURL, cfg.chunkSize)
	defer rc.GetClient().CloseIdleConnections()

	boardID := createBoard(rc)
	players := createPlayers(rc, cfg.playerCount)

	fmt.Printf(
		"board=%s players=%d chunkSize=%d chunkDelay=%s\n",
		boardID, len(players), cfg.chunkSize, cfg.chunkDelay,
	)

	started := time.Now()

	errs := runFanoutSets(rc, boardID, players, cfg.chunkSize, cfg.chunkDelay)

	fmt.Printf("set %d scores in %s: failed=%d\n", len(players)-len(errs), time.Since(started), len(errs))
	for i, err := range errs {
		if i >= 20 {
			fmt.Printf("... %d additional errors omitted\n", len(errs)-20)
			break
		}
		fmt.Printf("error: %v\n", err)
	}
	if len(errs) != 0 {
		log.Fatalf("fan-out writes failed")
	}

	rows, total := fetchAllRows(rc, boardID)
	printRankings(rows, rankingsShown)

	verifyRanking(rc, boardID, players, rows, total)
	verifyProjection(rc, boardID)

	// rows[0] is the verified top-ranked player; persist its ledger as a run artifact.
	history, err := apexhttp.FetchHistory(rc, boardID, rows[0].PlayerID)
	if err != nil {
		log.Fatalf("top player history: %v", err)
	}
	if err := apexhttp.SaveHistoryToFile(history, cfg.historyOut); err != nil {
		log.Fatalf("save history: %v", err)
	}
	fmt.Printf("saved %d history events for the top player to %s\n", len(history.Events), cfg.historyOut)

	fmt.Printf("OK: %d players ranked correctly, projection clean\n", len(players))
}

// printRankings prints the top rows of the leaderboard so a run shows the actual order,
// not just a pass/fail line.
func printRankings(rows []playerStanding, limit int) {
	shown := min(limit, len(rows))
	fmt.Printf("rankings (top %d of %d):\n", shown, len(rows))
	for _, r := range rows[:shown] {
		fmt.Printf("  #%-3d score=%-6g %s\n", r.Rank, r.Score, r.PlayerID)
	}
	if len(rows) > shown {
		fmt.Printf("  ... %d more\n", len(rows)-shown)
	}
}

// runFanoutSets sets each player's score concurrently, in chunks. Returns write errors.
func runFanoutSets(rc *resty.Client, boardID string, players []player, chunkSize int, chunkDelay time.Duration) []error {
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	for start := 0; start < len(players); start += chunkSize {
		end := min(start+chunkSize, len(players))
		for _, p := range players[start:end] {
			wg.Add(1)
			go func(p player) {
				defer wg.Done()
				if err := apexhttp.SetScore(rc, boardID, p.id, p.score); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}(p)
		}
		if end < len(players) {
			time.Sleep(chunkDelay)
		}
	}
	wg.Wait()
	return errs
}

func verifyRanking(rc *resty.Client, boardID string, players []player, rows []playerStanding, total int) {
	if total != len(players) {
		log.Fatalf("total mismatch: got %d, want %d", total, len(players))
	}
	if len(rows) != len(players) {
		log.Fatalf("row count mismatch: got %d, want %d", len(rows), len(players))
	}

	// Expected order is players sorted by score, highest first (scores are distinct).
	expected := append([]player(nil), players...)
	sort.Slice(expected, func(a, b int) bool { return expected[a].score > expected[b].score })

	for i, row := range rows {
		if row.Rank != int64(i+1) {
			log.Fatalf("rank not contiguous at index %d: got rank %d, want %d", i, row.Rank, i+1)
		}
		if row.PlayerID != expected[i].id {
			log.Fatalf("wrong player at rank %d: got %s, want %s", i+1, row.PlayerID, expected[i].id)
		}
		if row.Score != expected[i].score {
			log.Fatalf("wrong score at rank %d: got %g, want %g", i+1, row.Score, expected[i].score)
		}
		if i > 0 && rows[i-1].Score <= row.Score {
			log.Fatalf("scores not strictly descending at rank %d", i+1)
		}
	}

	// Cross-check the single-standing read path against the listing for a sample of positions
	// (first, middle, last) rather than re-reading all N players.
	for _, idx := range []int{0, len(rows) / 2, len(rows) - 1} {
		want := rows[idx]
		got := fetchStanding(rc, boardID, want.PlayerID)
		if got.Rank != want.Rank || got.Score != want.Score {
			log.Fatalf(
				"standing disagrees with listing for %s: standing rank=%d score=%g, listing rank=%d score=%g",
				want.PlayerID, got.Rank, got.Score, want.Rank, want.Score,
			)
		}
	}
}
