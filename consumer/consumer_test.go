package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/salandered/apex/ledger"
	"github.com/stretchr/testify/require"
)

func TestConsumerBuildsDailyCountsAndStartsAtLedgerHead(t *testing.T) {
	day1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 16, 3, 0, 0, 0, time.UTC)
	store := &fakeStore{batch: LedgerBatch{
		Events: []ledger.Event{
			{ID: "1-0", PlayerID: "alice", CreatedAt: day1},
			{ID: "2-0", PlayerID: "bob", CreatedAt: day1.Add(time.Minute)},
			{ID: "3-0", PlayerID: "alice", CreatedAt: day2},
		},
		LastID: "3-0",
	}}

	n, err := NewDailyActivityConsumer(store).processOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, cursorHead, store.readAfter)
	require.Equal(t, int64(batchCount), store.readLimit)
	require.Equal(t, blockDuration, store.readBlock)
	require.Equal(t, []DailyIncrement{
		{Date: "2026-01-15", PlayerID: "alice", Count: 1},
		{Date: "2026-01-15", PlayerID: "bob", Count: 1},
		{Date: "2026-01-16", PlayerID: "alice", Count: 1},
	}, store.applied)
	require.Equal(t, dailyTTL, store.appliedTTL)
	require.Equal(t, "3-0", store.cursor)
}

func TestConsumerResumesFromPersistedCursor(t *testing.T) {
	store := &fakeStore{
		cursor:      "10-0",
		cursorFound: true,
		batch: LedgerBatch{
			Events: []ledger.Event{{ID: "11-0", PlayerID: "alice", CreatedAt: time.Now().UTC()}},
			LastID: "11-0",
		},
	}

	_, err := NewDailyActivityConsumer(store).processOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, "10-0", store.readAfter)
	require.Equal(t, "11-0", store.cursor)
}

func TestConsumerSkipsRejectedEntriesAndAdvancesCursor(t *testing.T) {
	store := &fakeStore{batch: LedgerBatch{
		Rejected: []RejectedEntry{{ID: "12-0", Err: errors.New("malformed")}},
		LastID:   "12-0",
	}}

	n, err := NewDailyActivityConsumer(store).processOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Empty(t, store.applied)
	require.Equal(t, "12-0", store.cursor)
}

func TestConsumerAppliesBeforeSavingCursor(t *testing.T) {
	store := &fakeStore{
		batch: LedgerBatch{
			Events: []ledger.Event{{ID: "13-0", PlayerID: "alice", CreatedAt: time.Now().UTC()}},
			LastID: "13-0",
		},
		saveErr: errors.New("save failed"),
	}

	n, err := NewDailyActivityConsumer(store).processOnce(context.Background())
	require.ErrorContains(t, err, "persist cursor")
	require.Equal(t, 1, n)
	require.Len(t, store.applied, 1)
	require.False(t, store.cursorFound)
}

type fakeStore struct {
	cursor      string
	cursorFound bool
	batch       LedgerBatch
	readAfter   string
	readLimit   int64
	readBlock   time.Duration
	applied     []DailyIncrement
	appliedTTL  time.Duration
	saveErr     error
}

func (s *fakeStore) LoadCursor(context.Context, string) (string, bool, error) {
	return s.cursor, s.cursorFound, nil
}

func (s *fakeStore) ReadLedgerBatch(
	_ context.Context, after string, limit int64, block time.Duration,
) (LedgerBatch, error) {
	s.readAfter = after
	s.readLimit = limit
	s.readBlock = block
	return s.batch, nil
}

func (s *fakeStore) ApplyDailyCounts(
	_ context.Context, increments []DailyIncrement, ttl time.Duration,
) error {
	s.applied = append(s.applied, increments...)
	s.appliedTTL = ttl
	return nil
}

func (s *fakeStore) SaveCursor(_ context.Context, _ string, cursor string) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.cursor = cursor
	s.cursorFound = true
	return nil
}
