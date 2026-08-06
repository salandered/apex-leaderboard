package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/salandered/apex/ledger"
	"github.com/stretchr/testify/require"
)

const testConsumerID = "test_view"

func TestConsumerAppliesBatchAndStartsAtLedgerHead(t *testing.T) {
	store := &fakeStore{batch: LedgerBatch{
		Events: []ledger.Event{
			{ID: "1-0", PlayerID: "alice"},
			{ID: "2-0", PlayerID: "bob"},
			{ID: "3-0", PlayerID: "alice"},
		},
		LastID: "3-0",
	}}
	consumer := mockedConsumer(store)

	// when
	n, err := consumer.processOnce(context.Background())

	// then
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, cursorHead, store.readAfter)
	require.Equal(t, int64(batchCount), store.readLimit)
	require.Equal(t, DefaultBlockDuration, store.readBlock)
	require.Equal(t, store.batch.Events, consumer.applied)
	require.Equal(t, testConsumerID, store.loadedCursorID)
	require.Equal(t, testConsumerID, store.savedCursorID)
	require.Equal(t, "3-0", store.cursor)
}

func TestConsumerResumesFromPersistedCursor(t *testing.T) {
	store := &fakeStore{
		cursor:      "10-0",
		cursorFound: true,
		batch: LedgerBatch{
			Events: []ledger.Event{{ID: "11-0", PlayerID: "alice"}},
			LastID: "11-0",
		},
	}

	_, err := mockedConsumer(store).processOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, "10-0", store.readAfter)
	require.Equal(t, "11-0", store.cursor)
}

func TestConsumerSkipsRejectedEntriesAndAdvancesCursor(t *testing.T) {
	store := &fakeStore{batch: LedgerBatch{
		Rejected: []RejectedEntry{{ID: "12-0", Err: errors.New("malformed")}},
		LastID:   "12-0",
	}}
	consumer := mockedConsumer(store)

	n, err := consumer.processOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Empty(t, consumer.applied)
	require.Equal(t, "12-0", store.cursor)
}

func TestConsumerAppliesBeforeSavingCursor(t *testing.T) {
	store := &fakeStore{
		batch: LedgerBatch{
			Events: []ledger.Event{{ID: "13-0", PlayerID: "alice"}},
			LastID: "13-0",
		},
		saveErr: errors.New("save failed"),
	}
	consumer := mockedConsumer(store)

	n, err := consumer.processOnce(context.Background())
	require.ErrorContains(t, err, "persist cursor")
	require.Equal(t, 1, n)
	require.Len(t, consumer.applied, 1)
	require.False(t, store.cursorFound)
}

func TestConsumerKeepsCursorWhenApplyFails(t *testing.T) {
	store := &fakeStore{batch: LedgerBatch{
		Events: []ledger.Event{{ID: "14-0", PlayerID: "alice"}},
		LastID: "14-0",
	}}
	consumer := mockedConsumer(store)
	consumer.applyErr = errors.New("apply failed")

	n, err := consumer.processOnce(context.Background())
	require.ErrorContains(t, err, "apply batch")
	require.Zero(t, n)
	require.Empty(t, store.savedCursorID)
	require.Empty(t, store.cursor)
	require.False(t, store.cursorFound)
}

// Consumer with a stubbed Apply recording every event it received.
type stubbedConsumer struct {
	*Consumer
	applied  []ledger.Event
	applyErr error // set before processOnce to fail the batch
}

func mockedConsumer(store ConsumerStore) *stubbedConsumer {
	stub := &stubbedConsumer{}
	stub.Consumer = &Consumer{
		store: store,
		ID:    testConsumerID,
		Apply: func(_ context.Context, events []ledger.Event) error {
			stub.applied = append(stub.applied, events...)
			return stub.applyErr
		},
		BlockDuration: DefaultBlockDuration,
	}
	return stub
}

type fakeStore struct {
	cursor         string
	cursorFound    bool
	loadedCursorID string
	savedCursorID  string
	batch          LedgerBatch
	readAfter      string
	readLimit      int64
	readBlock      time.Duration
	saveErr        error
}

func (s *fakeStore) LoadCursor(_ context.Context, consumer string) (string, bool, error) {
	s.loadedCursorID = consumer
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

func (s *fakeStore) SaveCursor(_ context.Context, consumer, cursor string) error {
	s.savedCursorID = consumer
	if s.saveErr != nil {
		return s.saveErr
	}
	s.cursor = cursor
	s.cursorFound = true
	return nil
}
