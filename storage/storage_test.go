//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

const testRedisImage = "redis:8.8.0-alpine"

var (
	mockedTime    = time.Date(2026, 1, 17, 12, 30, 0, 0, time.UTC)
	mockedTimeStr = apextime.Format(mockedTime)
)

type StorageSuite struct {
	suite.Suite
	storage       Storage
	activityStore *redisActivityStore
	rawClient     *redis.Client // for assertions + flushing
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

	s.storage, err = NewStorage(url)
	s.Require().NoError(err)

	opts, err := redis.ParseURL(url)
	s.Require().NoError(err)
	s.rawClient = redis.NewClient(opts)
	s.activityStore = newActivityStore(s.rawClient)
	s.T().Cleanup(func() { s.rawClient.Close() })
}

// Cleans up the db so tests stay order-independent.
func (s *StorageSuite) SetupTest() {
	ctx := s.ctx()
	s.Require().NoError(s.rawClient.FlushDB(ctx).Err())
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
	s.createBoard(board.MainId, "main", mockedTime)
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
