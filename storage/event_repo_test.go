//go:build integration

package storage

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

func (s *StorageSuite) TestListEventsAfterZero() {
	ctx := s.ctx()
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	playerID := player.GenerateID()
	firstID := s.addLedgerEntryAt(ctx, start, playerID, "r1")
	secondID := s.addLedgerEntryAt(ctx, start.Add(time.Minute), playerID, "r2")
	thirdID := s.addLedgerEntryAt(ctx, start.Add(2*time.Minute), playerID, "r3")

	events, err := s.storage.ListEventsAfter(ctx, "0-0", 10)

	s.Require().NoError(err)
	s.Require().Len(events, 3)
	s.Require().Equal(firstID, events[0].ID)
	s.Require().Equal(secondID, events[1].ID)
	s.Require().Equal(thirdID, events[2].ID)
}

func (s *StorageSuite) TestListEventsAfterExclusiveCursor() {
	ctx := s.ctx()
	start := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	playerID := player.GenerateID()
	firstID := s.addLedgerEntryAt(ctx, start, playerID, "r1")
	secondID := s.addLedgerEntryAt(ctx, start.Add(time.Minute), playerID, "r2")

	events, err := s.storage.ListEventsAfter(ctx, firstID, 10)

	s.Require().NoError(err)
	s.Require().Len(events, 1)
	s.Require().Equal(secondID, events[0].ID)
}

func (s *StorageSuite) TestListEventsAfterHonorsLimit() {
	ctx := s.ctx()
	start := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	playerID := player.GenerateID()
	s.addLedgerEntryAt(ctx, start, playerID, "r1")
	secondID := s.addLedgerEntryAt(ctx, start.Add(time.Minute), playerID, "r2")
	s.addLedgerEntryAt(ctx, start.Add(2*time.Minute), playerID, "r3")

	events, err := s.storage.ListEventsAfter(ctx, "0-0", 2)

	s.Require().NoError(err)
	s.Require().Len(events, 2)
	s.Require().Equal(secondID, events[1].ID)
}

func (s *StorageSuite) TestListEventsAfterEmpty() {
	events, err := s.storage.ListEventsAfter(s.ctx(), "0-0", 10)

	s.Require().NoError(err)
	s.Require().Empty(events)
}

func (s *StorageSuite) TestListEventsAfterMalformedEntryFails() {
	ctx := s.ctx()
	s.Require().NoError(s.rawClient.XAdd(ctx, &redis.XAddArgs{
		Stream: ledgerKey,
		Values: map[string]any{
			entryFieldType:      "unknown",
			entryFieldPlayerID:  string(player.GenerateID()),
			entryFieldBoardID:   string(board.MainId),
			entryFieldAmount:    "1",
			entryFieldRequestID: "r1",
		},
	}).Err())

	events, err := s.storage.ListEventsAfter(ctx, "0-0", 10)

	s.Require().ErrorIs(err, ErrInconsistent)
	s.Require().Nil(events)
}
