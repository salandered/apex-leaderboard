//go:build integration

package storage

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/salandered/apex/apexredis"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
)

const testRedisImage = "redis:8.8.0-alpine"

var (
	mockedTime             = time.Date(2026, 1, 17, 12, 30, 0, 0, time.UTC)
	mockedTimeStr          = apextime.Format(mockedTime)
	testBoardId   board.ID = "test-board"
)

type StorageSuite struct {
	suite.Suite
	storage            Storage
	activityStore      *redisActivityStore
	playerHistoryStore *redisPlayerHistoryStore
	ledgerConsumer     *redisLedgerConsumer
	rawClient          *redis.Client // for assertions + flushing
}

func TestStorageSuite(t *testing.T) {
	suite.Run(t, new(StorageSuite))
}

// launches Redis container (random host port)
func (s *StorageSuite) SetupSuite() {
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, testRedisImage)
	testcontainers.CleanupContainer(s.T(), ctr) // adds to s.T() Cleanup
	s.Require().NoError(err)

	url, err := ctr.ConnectionString(ctx)
	s.Require().NoError(err)

	// one client for all three, for tests it's fine
	s.rawClient, err = apexredis.New(apexredis.Config{URL: url}, "storage")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.rawClient.Close() })

	s.storage = NewStorage(s.rawClient)
	s.activityStore = NewActivityStore(s.rawClient)
	s.playerHistoryStore = NewPlayerHistoryStore(s.rawClient)
	s.ledgerConsumer = &redisLedgerConsumer{client: s.rawClient}
}

// Cleans up the db so tests stay order-independent.
func (s *StorageSuite) SetupTest() {
	ctx := s.ctx()
	s.Require().NoError(s.rawClient.FlushDB(ctx).Err())
}

// Concurrency tests
// TODO: experimental, probably e2e during CI

// N concurrent increments apply N ops and append N events.
func (s *StorageSuite) TestConcurrentIncrementScoreApplielAllOnce() {
	ctx := s.ctx()
	s.createMainBoard()
	playerId := s.createPlayer("alice")

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Require().NoError(
				s.storage.IncrementScore(ctx, playerId, testBoardId, 1, "r"+strconv.Itoa(i), ""),
			)
		}(i)
	}
	wg.Wait()

	score, err := s.rawClient.ZScore(ctx, leaderboardKey(testBoardId), string(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(float64(n), score)
	s.requireStreamLen(ctx, n)
}

// N concurrent creates: one wins, others result in ErrPlayerExists.
func (s *StorageSuite) TestConcurrentCreatePlayerProfileAppliedOne() {
	ctx := s.ctx()
	playerId := player.GenerateID()

	const n = 50
	var wg sync.WaitGroup
	var wins atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.storage.CreatePlayerProfile(
				ctx, &player.Profile{PlayerId: playerId, PlayerName: "alice"}, "",
			)
			if err == nil {
				wins.Add(1)
				return
			}
			s.Require().ErrorIs(err, ErrPlayerExists)
		}(i)
	}
	wg.Wait()

	s.Require().Equal(int64(1), wins.Load())

	profile, err := s.storage.GetPlayerProfile(ctx, playerId)
	s.Require().NoError(err)
	s.Require().Equal(playerId, profile.PlayerId)
}

// Utils

func (s *StorageSuite) createPlayer(name string) player.ID {
	playerId := player.GenerateID()
	ctx := s.ctx()
	err := s.rawClient.HSet(
		ctx,
		playerProfileKey(playerId),
		profileNameField, name,
		profileCreatedAtField, mockedTimeStr,
	).Err()
	s.Require().NoError(err)
	return playerId
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
			entryFieldBoardID:   string(testBoardId),
			entryFieldAmount:    "1",
			entryFieldRequestID: reqID,
		},
	}).Err()
	s.Require().NoError(err)
	return id
}

// asserts the ledger holds n events
func (s *StorageSuite) requireStreamLen(ctx context.Context, n int64) {
	actual, err := s.rawClient.XLen(ctx, ledgerKey).Result()
	s.Require().NoError(err)
	s.Require().Equal(n, actual)
}

// returns the field/value map of the newest ledger entry
func (s *StorageSuite) lastEvent(ctx context.Context) map[string]string {
	entries, err := s.rawClient.XRevRangeN(ctx, ledgerKey, "+", "-", 1).Result()
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	out := make(map[string]string, len(entries[0].Values))
	for k, v := range entries[0].Values {
		out[k] = v.(string)
	}
	return out
}

// cancelled automatically when the test ends
func (s *StorageSuite) ctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	s.T().Cleanup(cancel)
	return ctx
}

func (s *StorageSuite) createMainBoard() {
	s.createBoard(testBoardId, "Test Board", mockedTime)
}

func (s *StorageSuite) createBoard(id board.ID, name string, createdAt time.Time) {
	// TODO use rawClient
	ctx := s.ctx()
	s.Require().NoError(s.storage.CreateBoard(ctx, &board.Board{
		BoardId:   id,
		BoardName: name,
		State:     board.BoardActive,
		CreatedAt: createdAt,
	}))
}

func (s *StorageSuite) closeBoard(id board.ID) {
	s.Require().NoError(s.storage.SetBoardState(s.ctx(), id, board.BoardClosed))
}

func (s *StorageSuite) requireEqualBoardHash(boardId board.ID, eName string, eCreatedAt string, eState board.BoardState) {
	fields, err := s.rawClient.HGetAll(s.ctx(), boardProfileKey(boardId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(map[string]string{
		boardNameField:      eName,
		boardCreatedAtField: eCreatedAt,
		boardStateField:     string(eState),
	},
		fields,
	)
}

func (s *StorageSuite) requireEqualPlayerHash(playerId player.ID, eName string, eCreatedAt string) {
	fields, err := s.rawClient.HGetAll(s.ctx(), playerProfileKey(playerId)).Result()
	s.Require().NoError(err)
	s.Require().Equal(map[string]string{
		profileNameField:      eName,
		profileCreatedAtField: eCreatedAt,
	},
		fields,
	)
}

func (s *StorageSuite) requireEqualBoardRegistry(eboardIds []string) {
	boardIds, err := s.rawClient.ZRange(s.ctx(), boardIndexKey, 0, -1).Result()
	s.Require().NoError(err)
	s.Require().ElementsMatch(eboardIds, boardIds)
}

// Points the default logger at a buffer for the duration of the test.
func captureLogs(t *testing.T, level slog.Leveler) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}
