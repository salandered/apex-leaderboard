package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/salandered/apex/loadtest/apexhttp"
)

const (
	activityLimit = 100
	pollInterval  = 100 * time.Millisecond
)

type activityPlayer struct {
	name          string
	id            string
	expectedCount int64
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	timeout := flag.Duration("timeout", 15*time.Second, "maximum time to wait for the activity projection")
	flag.Parse()

	if *timeout <= 0 {
		log.Fatal("timeout must be positive")
	}

	rc := apexhttp.NewClient(*baseURL, 4)
	defer rc.GetClient().CloseIdleConnections()

	boardID := apexhttp.SeedBoardID("daily-activity")
	if err := apexhttp.CreateBoard(rc, boardID, "Daily Activity Test"); err != nil {
		log.Fatalf("create board: %v", err)
	}

	players := []activityPlayer{
		{expectedCount: 30},
		{expectedCount: 20},
		{expectedCount: 10},
	}
	for i := range players {
		// the expected count is part of the name, so a failure log points at the fixture
		players[i].name = apexhttp.PlayerName(i, strconv.FormatInt(players[i].expectedCount, 10))
		playerID, err := apexhttp.CreatePlayer(rc, players[i].name)
		if err != nil {
			log.Fatalf("create player %q: %v", players[i].name, err)
		}
		players[i].id = playerID
	}

	date := time.Now().UTC().Format(time.DateOnly)
	for _, player := range players {
		if err := writeActivity(rc, boardID, player); err != nil {
			log.Fatalf("write activity for %s: %v", player.id, err)
		}
	}
	if currentDate := time.Now().UTC().Format(time.DateOnly); currentDate != date {
		log.Fatalf("UTC date changed from %s to %s during the writes; rerun the script", date, currentDate)
	}

	fmt.Printf("board=%s date=%s players=%d\n", boardID, date, len(players))

	activity, err := waitForActivity(rc, date, players, *timeout)
	if err != nil {
		log.Fatal(err)
	}
	verifyFixtureOrder(activity, players)

	positions := entryPositions(activity.Entries)
	for _, player := range players {
		position := positions[player.id]
		fmt.Printf(
			"  position=%d count=%d player=%s\n",
			position+1,
			activity.Entries[position].Count,
			player.id,
		)
	}
	fmt.Printf("OK: /api/v1/activity/daily returned today's expected activity counts\n")
}

func writeActivity(rc *resty.Client, boardID string, player activityPlayer) error {
	if err := apexhttp.SetScore(rc, boardID, player.id, 0); err != nil {
		return fmt.Errorf("set score: %w", err)
	}

	for i := int64(1); i < player.expectedCount; i++ {
		if err := apexhttp.IncrementScore(rc, boardID, player.id, 1); err != nil {
			return fmt.Errorf("increment %d: %w", i, err)
		}
	}
	return nil
}

func waitForActivity(
	rc *resty.Client,
	date string,
	players []activityPlayer,
	timeout time.Duration,
) (apexhttp.ListDailyActivityResp, error) {
	deadline := time.Now().Add(timeout)
	for {
		activity, err := apexhttp.FetchDailyActivity(rc, date, activityLimit)
		if err != nil {
			return apexhttp.ListDailyActivityResp{}, fmt.Errorf("list daily activity: %w", err)
		}
		if activity.Date != date {
			return apexhttp.ListDailyActivityResp{}, fmt.Errorf("response date mismatch: got %q, want %q", activity.Date, date)
		}

		ready, err := hasExpectedCounts(activity.Entries, players)
		if err != nil {
			return apexhttp.ListDailyActivityResp{}, err
		}
		if ready {
			return activity, nil
		}
		if time.Now().After(deadline) {
			return apexhttp.ListDailyActivityResp{}, fmt.Errorf(
				"timed out after %s waiting for today's activity projection; last fixture counts: %s",
				timeout,
				formatFixtureCounts(activity.Entries, players),
			)
		}
		time.Sleep(pollInterval)
	}
}

func hasExpectedCounts(entries []apexhttp.ActivityEntry, players []activityPlayer) (bool, error) {
	counts := make(map[string]int64, len(entries))
	for _, entry := range entries {
		counts[entry.PlayerID] = entry.Count
	}

	for _, player := range players {
		count, found := counts[player.id]
		if !found || count < player.expectedCount {
			return false, nil
		}
		if count > player.expectedCount {
			return false, fmt.Errorf(
				"activity count for fresh player %s is %d, want %d",
				player.id,
				count,
				player.expectedCount,
			)
		}
	}
	return true, nil
}

func verifyFixtureOrder(activity apexhttp.ListDailyActivityResp, players []activityPlayer) {
	positions := entryPositions(activity.Entries)
	for i := 1; i < len(players); i++ {
		previous := players[i-1]
		current := players[i]
		if positions[previous.id] >= positions[current.id] {
			log.Fatalf(
				"activity order mismatch: player %s with count %d follows player %s with count %d",
				previous.id,
				previous.expectedCount,
				current.id,
				current.expectedCount,
			)
		}
	}
}

func entryPositions(entries []apexhttp.ActivityEntry) map[string]int {
	positions := make(map[string]int, len(entries))
	for i, entry := range entries {
		positions[entry.PlayerID] = i
	}
	return positions
}

func formatFixtureCounts(entries []apexhttp.ActivityEntry, players []activityPlayer) string {
	counts := make(map[string]int64, len(entries))
	for _, entry := range entries {
		counts[entry.PlayerID] = entry.Count
	}

	result := make([]string, 0, len(players))
	for _, player := range players {
		result = append(result, fmt.Sprintf("%s=%d/%d", player.id, counts[player.id], player.expectedCount))
	}
	return strings.Join(result, ", ")
}
