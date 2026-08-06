//go:build integration

package storage

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/player"
)

const testConsumerName = "test_view"

func (s *StorageSuite) TestLoadCursorMissing() {
	cursor, found, err := s.ledgerConsumer.LoadCursor(s.ctx(), testConsumerName)
	s.Require().NoError(err)
	s.Require().False(found)
	s.Require().Empty(cursor)
}

func (s *StorageSuite) TestLoadCursorExisting() {
	ctx := s.ctx()
	s.Require().NoError(
		s.rawClient.Set(ctx, consumerCursorKey(testConsumerName), "123-4", 0).Err(),
	)

	cursor, found, err := s.ledgerConsumer.LoadCursor(ctx, testConsumerName)

	s.Require().NoError(err)
	s.Require().True(found)
	s.Require().Equal("123-4", cursor)
}

func (s *StorageSuite) TestSaveCursor() {
	ctx := s.ctx()

	err := s.ledgerConsumer.SaveCursor(ctx, testConsumerName, "123-4")

	s.Require().NoError(err)
	cursor, err := s.rawClient.Get(ctx, consumerCursorKey(testConsumerName)).Result()
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

	batch, err := s.ledgerConsumer.ReadLedgerBatch(ctx, "0-0", 10, time.Second)

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

	batch, err := s.ledgerConsumer.ReadLedgerBatch(ctx, firstID, 10, time.Second)

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

	batch, err := s.ledgerConsumer.ReadLedgerBatch(ctx, "0-0", 2, time.Second)

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
			entryFieldBoardID:   string(testBoardId),
			entryFieldAmount:    "1",
			entryFieldRequestID: "r1",
		},
	}).Result()
	s.Require().NoError(err)

	batch, err := s.ledgerConsumer.ReadLedgerBatch(ctx, "0-0", 10, time.Second)

	s.Require().NoError(err)
	s.Require().Empty(batch.Events)
	s.Require().Len(batch.Rejected, 1)
	s.Require().Equal(id, batch.Rejected[0].ID)
	s.Require().Equal(id, batch.LastID)
}
