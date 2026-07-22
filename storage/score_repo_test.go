//go:build integration

package storage

import (
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

// unregistered board id, used by the "board missing" cases
const missingBoardId = board.ID("no-such-board")

func (s *StorageSuite) TestSetScore() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob") // profile only, no score yet

	// when
	err := s.storage.SetScore(ctx, playerId, board.MainId, 100.0, "r-set", "")

	// then
	s.Require().NoError(err)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(100.0, score)

	s.requireStreamLen(ctx, 1)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(ledger.EventSet), last[entryFieldType])
	s.Require().Equal("100", last[entryFieldAmount])
}

// the script checks the player before the board, so the player error wins
func (s *StorageSuite) TestSetScorePlayerAndBoardMissing() {
	ctx := s.ctx()

	// when
	err := s.storage.SetScore(ctx, player.GenerateID(), missingBoardId, 100.0, "r-set", "")

	// then
	s.Require().ErrorIs(err, ErrNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestSetScorePlayerMissing() {
	ctx := s.ctx()

	s.createMainBoard()

	// when
	err := s.storage.SetScore(ctx, player.GenerateID(), board.MainId, 100.0, "r-set", "")

	// then
	s.Require().ErrorIs(err, ErrNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestSetScoreBoardMissing() {
	ctx := s.ctx()

	playerId := s.createPlayer("bob")

	// when
	err := s.storage.SetScore(ctx, playerId, missingBoardId, 100.0, "r-set", "")

	// then
	s.Require().ErrorIs(err, ErrBoardNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestIncrementScore() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	// when
	err := s.storage.IncrementScore(ctx, playerId, board.MainId, 5.0, "r-inc", "")

	// then
	s.Require().NoError(err)

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(5.0, score) // increment starts from 0

	s.requireStreamLen(ctx, 1)
	last := s.lastEvent(ctx)
	s.Require().Equal(string(ledger.EventIncrement), last[entryFieldType])
	s.Require().Equal("5", last[entryFieldAmount])
}

// the script checks the player before the board, so the player error wins
func (s *StorageSuite) TestIncrementScorePlayerAndBoardMissing() {
	ctx := s.ctx()

	// when
	err := s.storage.IncrementScore(ctx, player.GenerateID(), missingBoardId, 5.0, "r-inc", "")

	// then
	s.Require().ErrorIs(err, ErrNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestIncrementScorePlayerMissing() {
	ctx := s.ctx()

	s.createMainBoard()

	// when
	err := s.storage.IncrementScore(ctx, player.GenerateID(), board.MainId, 5.0, "r-inc", "")

	// then
	s.Require().ErrorIs(err, ErrNotFound)

	// a rejected write is not an event (rule 3)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestIncrementScoreBoardMissing() {
	ctx := s.ctx()

	playerId := s.createPlayer("bob")

	// when
	err := s.storage.IncrementScore(ctx, playerId, missingBoardId, 5.0, "r-inc", "")

	// then
	s.Require().ErrorIs(err, ErrBoardNotFound)
	s.requireStreamLen(ctx, 0)
}

func (s *StorageSuite) TestIncrementScoreSameIdempKeySamePayloadIsIdempotent() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	s.Require().NoError(
		s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "req-1", "idem-1"),
	)
	s.requireStreamLen(ctx, 1)

	s.Require().NoError(
		s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "req-2", "idem-1"),
	)
	s.requireStreamLen(ctx, 1) // no new event

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(10.0, score)
}

func (s *StorageSuite) TestIncrementScoreSameIdempKeyDiffPayloadShouldConflict() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "req-1", "idem-1"))
	s.requireStreamLen(ctx, 1)

	err := s.storage.IncrementScore(ctx, playerId, board.MainId, 20, "req-2", "idem-1")
	s.Require().ErrorIs(err, ErrIdempotencyConflict)
	s.requireStreamLen(ctx, 1) // conflict appends nothing

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(10.0, score)
}

func (s *StorageSuite) TestIncrementScoreNoIdempotencyKeyLeavesHashEmpty() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, board.MainId, 10, "req-1", ""))

	hlen, err := s.rawClient.HLen(ctx, idempotencyHashKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(0), hlen)
}

func (s *StorageSuite) TestIdempotencyRecordGetsTTL() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 10, "req-1", "idem-1"))

	field := string(board.MainId) + ":" + string(playerId) + ":idem-1"
	ttls, err := s.rawClient.HTTL(ctx, idempotencyHashKey, field).Result()
	s.Require().NoError(err)
	s.Require().Len(ttls, 1)
	s.Require().Greater(ttls[0], int64(0)) // remaining seconds in the 24h bucket
}

func (s *StorageSuite) TestScoreOperationSequence() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 20, "r1", ""))

	s.incrementScore(playerId, 1, "r2")
	s.incrementScore(playerId, -6, "r3")

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 50, "r4", ""))

	s.incrementScore(playerId, 10, "r5")
	s.incrementScore(playerId, -4, "r6")

	projected, err := s.rawClient.ZScore(ctx, leaderboardKey(board.MainId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(56.0, projected)
	s.requireStreamLen(ctx, 6)
}

func (s *StorageSuite) TestListScores() {
	ctx := s.ctx()

	s.createMainBoard()

	seeds := []struct {
		name  string
		score float64
	}{{"alice", 30}, {"bob", 20}, {"carol", 10}}
	ids := make(map[string]player.ID, len(seeds))
	for i, sd := range seeds {
		id := player.GenerateID()
		_, err := s.storage.CreatePlayerProfile(ctx,
			&player.Profile{PlayerId: id, PlayerName: sd.name}, "")
		s.Require().NoError(err)
		s.Require().NoError(s.storage.SetScore(ctx, id, board.MainId, sd.score, "ls-set"+strconv.Itoa(i), ""))
		ids[sd.name] = id
	}

	// first page: highest first, ranks 1..2; total is the whole board
	page, total, err := s.storage.ListStandings(ctx, board.MainId, 2, 0)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(page, 2)
	s.Require().Equal(ids["alice"].String(), page[0].PlayerID)
	s.Require().Equal(30.0, page[0].Score)
	s.Require().Equal(int64(1), page[0].Rank)
	s.Require().Equal(ids["bob"].String(), page[1].PlayerID)
	s.Require().Equal(int64(2), page[1].Rank)

	// second page continues the ranking
	page2, total, err := s.storage.ListStandings(ctx, board.MainId, 2, 2)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(page2, 1)
	s.Require().Equal(ids["carol"].String(), page2[0].PlayerID)
	s.Require().Equal(int64(3), page2[0].Rank)

	// offset past the end -> empty slice, total still reports the board size
	empty, total, err := s.storage.ListStandings(ctx, board.MainId, 10, 5)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Empty(empty)
}

func (s *StorageSuite) TestGetStanding() {
	ctx := s.ctx()

	s.createMainBoard()

	// when
	seeds := []struct {
		name  string
		score float64
	}{{"alice", 30}, {"bob", 20}, {"carol", 10}}
	ids := make(map[string]player.ID, len(seeds))
	for i, sd := range seeds {
		id := player.GenerateID()
		_, err := s.storage.CreatePlayerProfile(ctx,
			&player.Profile{PlayerId: id, PlayerName: sd.name}, "")
		s.Require().NoError(err)
		s.Require().NoError(s.storage.SetScore(
			ctx, id, board.MainId, sd.score, "pr-set"+strconv.Itoa(i), ""),
		)
		ids[sd.name] = id
	}

	// then
	// top player is rank 1
	standing, total, err := s.storage.GetStanding(ctx, ids["alice"], board.MainId)
	s.Require().NoError(err)
	s.Require().Equal(ids["alice"].String(), standing.PlayerID)
	s.Require().Equal(int64(1), standing.Rank)
	s.Require().Equal(30.0, standing.Score)
	s.Require().Equal(int64(3), total)

	// a mid-board player
	standing, total, err = s.storage.GetStanding(ctx, ids["bob"], board.MainId)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), standing.Rank)
	s.Require().Equal(20.0, standing.Score)
	s.Require().Equal(int64(3), total)

	// unranked player -> not found
	_, total, err = s.storage.GetStanding(ctx, player.GenerateID(), board.MainId)
	s.Require().ErrorIs(err, ErrNotFound)
	s.Require().Equal(int64(0), total) // default value
}

// the same player's scores on two boards move independently over one ledger
func (s *StorageSuite) TestTwoBoardsOnePlayer() {
	ctx := s.ctx()

	s.createMainBoard()
	playerId := s.createPlayer("bob")
	s.createBoard("weekly", "Weekly", mockedTime)

	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 10, "r1", ""))
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.ID("weekly"), 100, "r2", ""))
	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, board.ID("weekly"), 5, "r3", ""))

	mainStanding, mainTotal, err := s.storage.GetStanding(ctx, playerId, board.MainId)
	s.Require().NoError(err)
	s.Require().Equal(10.0, mainStanding.Score)
	s.Require().Equal(int64(1), mainTotal)

	weeklyStanding, _, err := s.storage.GetStanding(ctx, playerId, board.ID("weekly"))
	s.Require().NoError(err)
	s.Require().Equal(105.0, weeklyStanding.Score)

	// per-board history: the shared request ids never cross board boundaries
	weeklyHistory, err := s.storage.PlayerHistory(ctx, playerId, board.ID("weekly"), 0)
	s.Require().NoError(err)
	s.Require().Len(weeklyHistory, 2)
	mainHistory, err := s.storage.PlayerHistory(ctx, playerId, board.MainId, 0)
	s.Require().NoError(err)
	s.Require().Len(mainHistory, 1)
}

func (s *StorageSuite) incrementScore(playerId player.ID, amount float64, reqID string) {
	ctx := s.ctx()
	s.Require().NoError(s.storage.IncrementScore(ctx, playerId, board.MainId, amount, reqID, ""))
}

func (s *StorageSuite) TestHistory() {
	ctx := s.ctx()

	s.createMainBoard()

	aliceId := player.GenerateID()
	_, err := s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: aliceId, PlayerName: "alice"}, "")
	s.Require().NoError(err)
	s.Require().NoError(s.storage.SetScore(ctx, aliceId, board.MainId, 5, "a1", ""))
	s.Require().NoError(s.storage.IncrementScore(ctx, aliceId, board.MainId, 3, "a2", ""))
	s.Require().NoError(s.storage.IncrementScore(ctx, aliceId, board.MainId, 10, "a3", ""))

	// a second player must not leak into alice's history
	bob := player.GenerateID()
	_, err = s.storage.CreatePlayerProfile(ctx, &player.Profile{PlayerId: bob, PlayerName: "bob"}, "")
	s.Require().NoError(err)

	// all alice events, newest first
	all, err := s.storage.PlayerHistory(ctx, aliceId, board.MainId, 0)
	s.Require().NoError(err)
	s.Require().Len(all, 3)
	s.Require().Equal(ledger.EventIncrement, all[0].Type)
	s.Require().Equal(10.0, all[0].Amount)
	s.Require().Equal("a3", all[0].RequestID)
	s.Require().Equal(ledger.EventSet, all[2].Type)
	s.Require().Equal(aliceId.String(), all[0].PlayerID)
	s.Require().False(all[0].CreatedAt.IsZero())

	// limit caps the result
	limited, err := s.storage.PlayerHistory(ctx, aliceId, board.MainId, 2)
	s.Require().NoError(err)
	s.Require().Len(limited, 2)
	s.Require().Equal("a3", limited[0].RequestID)

	// unknown player yields an empty (non-nil) slice
	none, err := s.storage.PlayerHistory(ctx, player.GenerateID(), board.MainId, 0)
	s.Require().NoError(err)
	s.Require().Empty(none)
}

func (s *StorageSuite) TestHistoryRejectsMalformedMatchingEvent() {
	ctx := s.ctx()
	s.createMainBoard()
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:      "unknown",
			entryFieldPlayerID:  string(playerId),
			entryFieldBoardID:   string(board.MainId),
			entryFieldAmount:    "5",
			entryFieldRequestID: "broken",
		},
	}).Err())

	events, err := s.storage.PlayerHistory(ctx, playerId, board.MainId, 0)

	s.Require().ErrorIs(err, ErrInconsistent)
	s.Require().Nil(events)
}

func (s *StorageSuite) TestHistoryIgnoresMalformedEventsOutsideRequestedScope() {
	ctx := s.ctx()
	s.createMainBoard()
	playerId := s.createPlayer("alice")
	s.Require().NoError(s.storage.SetScore(ctx, playerId, board.MainId, 20, "r1", ""))
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:     "unknown",
			entryFieldPlayerID: string(playerId),
			entryFieldBoardID:  "weekly",
			entryFieldAmount:   "not-a-number",
		},
	}).Err())

	events, err := s.storage.PlayerHistory(ctx, playerId, board.MainId, 0)

	s.Require().NoError(err)
	s.Require().Len(events, 1)
	s.Require().Equal("r1", events[0].RequestID)
}

func (s *StorageSuite) TestListStandingsAsOfStopsAtExclusiveCutoff() {
	ctx := s.ctx()
	playerID := player.GenerateID()
	fistDay := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	nextDay := fistDay.AddDate(0, 0, 1)
	firstDayVeryEnd := nextDay.Add(-time.Millisecond)
	initialScore := 10.0
	firstDayVeryEndIncr := 5.0
	nextDayIncr := 100.0

	s.appendHistoricalEvent(
		streamIDAt(fistDay),
		ledger.EventSet,
		playerID,
		board.MainId,
		initialScore,
	)
	s.appendHistoricalEvent(
		streamIDAt(firstDayVeryEnd),
		ledger.EventIncrement,
		playerID,
		board.MainId,
		firstDayVeryEndIncr,
	)
	s.appendHistoricalEvent(
		streamIDAt(nextDay),
		ledger.EventIncrement,
		playerID,
		board.MainId,
		nextDayIncr,
	)

	standings, total, err := s.storage.ListStandingsAsOf(
		ctx, board.MainId, nextDay, 10, 0,
	)

	s.Require().NoError(err)
	s.Require().Equal(int64(1), total)
	s.Require().Equal(initialScore+firstDayVeryEndIncr, standings[0].Score) // nextDayIncr excluded
}

func (s *StorageSuite) TestListStandingsAsOfOrdersTiesLikeLiveLeaderboard() {
	alice := player.GenerateID()
	bob := player.GenerateID()
	s.appendHistoricalEvent("1-0", ledger.EventSet, alice, board.MainId, 20)
	s.appendHistoricalEvent("2-0", ledger.EventSet, bob, board.MainId, 20)

	standings, _, err := s.storage.ListStandingsAsOf(
		s.ctx(), board.MainId, time.UnixMilli(3), 10, 0,
	)

	s.Require().NoError(err)
	// TODO: looks weird, not determined kinda
	if alice.String() > bob.String() {
		s.Require().Equal(alice.String(), standings[0].PlayerID)
	} else {
		s.Require().Equal(bob.String(), standings[0].PlayerID)
	}
	s.Require().Equal(int64(1), standings[0].Rank)
	s.Require().Equal(int64(2), standings[1].Rank)
}

func (s *StorageSuite) TestListStandingsAsOfAppliesPaginationAfterFolding() {
	players := []player.ID{player.GenerateID(), player.GenerateID(), player.GenerateID()}
	for i, score := range []float64{30, 20, 10} {
		s.appendHistoricalEvent(
			[]string{"1-0", "2-0", "3-0"}[i],
			ledger.EventSet,
			players[i],
			board.MainId,
			score,
		)
	}

	standings, total, err := s.storage.ListStandingsAsOf(
		s.ctx(), board.MainId, time.UnixMilli(10), 1, 1,
	)

	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(standings, 1)
	s.Require().Equal(20.0, standings[0].Score)
	s.Require().Equal(int64(2), standings[0].Rank)
}

func (s *StorageSuite) TestListStandingsAsOfIgnoresLiveProjection() {
	playerID := player.GenerateID()
	s.appendHistoricalEvent("1-0", ledger.EventSet, playerID, board.MainId, 42)
	s.Require().NoError(s.rawClient.ZAdd(
		s.ctx(), leaderboardKey(board.MainId), redis.Z{Score: 999, Member: playerID.String()},
	).Err())

	standings, _, err := s.storage.ListStandingsAsOf(
		s.ctx(), board.MainId, time.UnixMilli(2), 10, 0,
	)

	s.Require().NoError(err)
	s.Require().Equal(42.0, standings[0].Score)
}

func (s *StorageSuite) TestListStandingsAsOfCurrentMatchesLiveProjection() {
	s.createMainBoard()
	alice := s.createPlayer("alice")
	bob := s.createPlayer("bob")
	s.Require().NoError(s.storage.SetScore(s.ctx(), alice, board.MainId, 20, "a1", ""))
	s.Require().NoError(s.storage.IncrementScore(s.ctx(), alice, board.MainId, 5, "a2", ""))
	s.Require().NoError(s.storage.SetScore(s.ctx(), bob, board.MainId, 25, "b1", ""))

	live, liveTotal, err := s.storage.ListStandings(s.ctx(), board.MainId, 10, 0)
	s.Require().NoError(err)
	historical, historicalTotal, err := s.storage.ListStandingsAsOf(
		s.ctx(), board.MainId, time.Now().UTC().Add(time.Second), 10, 0,
	)

	s.Require().NoError(err)
	s.Require().Equal(liveTotal, historicalTotal)
	s.Require().Equal(live, historical)
}

func (s *StorageSuite) TestListStandingsAsOfUnknownBoardIsEmpty() {
	standings, total, err := s.storage.ListStandingsAsOf(
		s.ctx(), "unknown", time.Now().UTC(), 10, 0,
	)

	s.Require().NoError(err)
	s.Require().Empty(standings)
	s.Require().Zero(total)
}

func (s *StorageSuite) TestListStandingsAsOfRejectsMalformedMatchingEvent() {
	playerID := player.GenerateID()
	s.Require().NoError(s.rawClient.XAdd(s.ctx(), &redis.XAddArgs{
		Stream: ledgerKey,
		ID:     "1-0",
		Values: map[string]any{
			entryFieldType:      string(ledger.EventIncrement),
			entryFieldPlayerID:  playerID.String(),
			entryFieldBoardID:   string(board.MainId),
			entryFieldAmount:    "not-a-number",
			entryFieldRequestID: "historical-test",
		},
	}).Err())

	standings, _, err := s.storage.ListStandingsAsOf(
		s.ctx(), board.MainId, time.UnixMilli(2), 10, 0,
	)

	s.Require().ErrorIs(err, ErrInconsistent)
	s.Require().Nil(standings)
}

func (s *StorageSuite) appendHistoricalEvent(
	id string, eventType ledger.EventType, playerID player.ID, boardID board.ID, amount float64,
) {
	s.Require().NoError(s.rawClient.XAdd(s.ctx(), &redis.XAddArgs{
		Stream: ledgerKey,
		ID:     id,
		Values: map[string]any{
			entryFieldType:      string(eventType),
			entryFieldPlayerID:  playerID.String(),
			entryFieldBoardID:   string(boardID),
			entryFieldAmount:    amount,
			entryFieldRequestID: "historical-test",
		},
	}).Err())
}

func streamIDAt(timestamp time.Time) string {
	return strconv.FormatInt(timestamp.UnixMilli(), 10) + "-0"
}
