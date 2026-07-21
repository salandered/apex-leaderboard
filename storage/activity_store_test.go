//go:build integration

package storage

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

const testDailyActivityConsumer = "daily_activity"

func (s *StorageSuite) TestLoadCursorMissing() {
	cursor, found, err := s.activityStore.LoadCursor(s.ctx(), testDailyActivityConsumer)
	s.Require().NoError(err)
	s.Require().False(found)
	s.Require().Empty(cursor)
}

func (s *StorageSuite) TestLoadCursorExisting() {
	ctx := s.ctx()
	s.Require().NoError(
		s.rawClient.Set(ctx, consumerCursorKey(testDailyActivityConsumer), "123-4", 0).Err(),
	)

	cursor, found, err := s.activityStore.LoadCursor(ctx, testDailyActivityConsumer)

	s.Require().NoError(err)
	s.Require().True(found)
	s.Require().Equal("123-4", cursor)
}

func (s *StorageSuite) TestSaveCursor() {
	ctx := s.ctx()

	err := s.activityStore.SaveCursor(ctx, testDailyActivityConsumer, "123-4")

	s.Require().NoError(err)
	cursor, err := s.rawClient.Get(ctx, consumerCursorKey(testDailyActivityConsumer)).Result()
	s.Require().NoError(err)
	s.Require().Equal("123-4", cursor)
}

func (s *StorageSuite) TestReadLedgerBatchAfterZero() {
	ctx := s.ctx()
	day1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 16, 3, 0, 0, 0, time.UTC)
	alice := player.GenerateID()
	bob := player.GenerateID()

	s.addLedgerEntryAt(ctx, day1, alice, "r1")
	s.addLedgerEntryAt(ctx, day1.Add(time.Minute), bob, "r2")
	s.addLedgerEntryAt(ctx, day1.Add(2*time.Minute), alice, "r3")
	lastID := s.addLedgerEntryAt(ctx, day2, alice, "r4")

	batch, err := s.activityStore.ReadLedgerBatch(ctx, "0-0", 10, time.Second)

	s.Require().NoError(err)
	s.Require().Len(batch.Events, 4)
	s.Require().Empty(batch.Rejected)
	s.Require().Equal(lastID, batch.LastID)
}

func (s *StorageSuite) TestReadLedgerBatchAfterCursor() {
	ctx := s.ctx()
	day := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	alice := player.GenerateID()

	firstID := s.addLedgerEntryAt(ctx, day, alice, "r1")
	secondID := s.addLedgerEntryAt(ctx, day.Add(time.Minute), alice, "r2")
	lastID := s.addLedgerEntryAt(ctx, day.Add(2*time.Minute), alice, "r3")

	batch, err := s.activityStore.ReadLedgerBatch(ctx, firstID, 10, time.Second)

	s.Require().NoError(err)
	s.Require().Len(batch.Events, 2)
	s.Require().Equal(secondID, batch.Events[0].ID)
	s.Require().Equal(lastID, batch.LastID)
}

func (s *StorageSuite) TestReadLedgerBatchLimit() {
	ctx := s.ctx()
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	alice := player.GenerateID()

	s.addLedgerEntryAt(ctx, day, alice, "r1")
	secondID := s.addLedgerEntryAt(ctx, day.Add(time.Minute), alice, "r2")
	s.addLedgerEntryAt(ctx, day.Add(2*time.Minute), alice, "r3")

	batch, err := s.activityStore.ReadLedgerBatch(ctx, "0-0", 2, time.Second)

	s.Require().NoError(err)
	s.Require().Len(batch.Events, 2)
	s.Require().Equal(secondID, batch.LastID)
}

func (s *StorageSuite) TestReadLedgerBatchRejectsMalformedEntry() {
	ctx := s.ctx()
	id, err := s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:      "unknown",
			entryFieldPlayerID:  string(player.GenerateID()),
			entryFieldBoardID:   string(board.MainId),
			entryFieldAmount:    "1",
			entryFieldRequestID: "r1",
		},
	}).Result()
	s.Require().NoError(err)

	batch, err := s.activityStore.ReadLedgerBatch(ctx, "0-0", 10, time.Second)

	s.Require().NoError(err)
	s.Require().Empty(batch.Events)
	s.Require().Len(batch.Rejected, 1)
	s.Require().Equal(id, batch.Rejected[0].ID)
	s.Require().Equal(id, batch.LastID)
}

func (s *StorageSuite) TestApplyDailyCountsCreatesMissingKey() {
	ctx := s.ctx()
	playerID := string(player.GenerateID())

	err := s.activityStore.ApplyDailyCounts(ctx, []consumer.DailyIncrement{
		{Date: "2026-04-01", PlayerID: playerID, Count: 2},
	}, 30*24*time.Hour)

	s.Require().NoError(err)
	score, err := s.rawClient.ZScore(ctx, activityDailyKey("2026-04-01"), playerID).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(2), score)
}

func (s *StorageSuite) TestApplyDailyCountsIncrementsExistingMember() {
	ctx := s.ctx()
	playerID := string(player.GenerateID())
	key := activityDailyKey("2026-04-02")
	s.Require().NoError(s.rawClient.ZAdd(ctx, key, redis.Z{Member: playerID, Score: 2}).Err())

	err := s.activityStore.ApplyDailyCounts(ctx, []consumer.DailyIncrement{
		{Date: "2026-04-02", PlayerID: playerID, Count: 1},
	}, 30*24*time.Hour)

	s.Require().NoError(err)
	score, err := s.rawClient.ZScore(ctx, key, playerID).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(3), score)
}

func (s *StorageSuite) TestApplyDailyCountsSetsTTL() {
	ctx := s.ctx()
	const ttl = 30 * 24 * time.Hour

	err := s.activityStore.ApplyDailyCounts(ctx, []consumer.DailyIncrement{
		{Date: "2026-04-03", PlayerID: string(player.GenerateID()), Count: 1},
	}, ttl)

	s.Require().NoError(err)
	actual, err := s.rawClient.TTL(ctx, activityDailyKey("2026-04-03")).Result()
	s.Require().NoError(err)
	s.Require().Greater(actual, ttl-time.Minute)
	s.Require().LessOrEqual(actual, ttl)
}

func (s *StorageSuite) addLedgerEntryAt(
	ctx context.Context, t time.Time, playerId player.ID, reqID string,
) string {
	// creates an explicit stream ID like '1768471200000-1'
	id := strconv.FormatInt(t.UnixMilli(), 10) + "-1"
	err := s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		ID:     id,
		Values: map[string]any{
			entryFieldType:      string(ledger.EventIncrement),
			entryFieldPlayerID:  string(playerId),
			entryFieldBoardID:   string(board.MainId),
			entryFieldAmount:    "1",
			entryFieldRequestID: reqID,
		},
	}).Err()
	s.Require().NoError(err)
	return id
}
