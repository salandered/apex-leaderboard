package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/salandered/apex/apispec"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/server"
	"github.com/salandered/apex/storage"
	"github.com/stretchr/testify/suite"
)

var MockedPlayerId = "698057b7-eb86-4f63-a228-100304c6ca0a"
var MockedBoardId = "main"
var MockedUnknownBoardId = "ghost-board"          // the mock answers ErrBoardNotFound
var MockedClosedBoardId = "closed-board"          // the mock rejects score writes with ErrBoardClosed
var MockedConflictIdempotencyKey = "conflict-key" // the mock answers ErrIdempotencyConflict on player create

func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APISuite))
}

type APISuite struct {
	suite.Suite
	server *httptest.Server
	client *http.Client
	router routers.Router
}

func (s *APISuite) SetupSuite() {
	fmt.Println("SetupSuite")
	s.server = httptest.NewServer(server.NewMux(getMockedStorage()))
	s.client = s.server.Client()

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(apispec.Spec)
	s.Require().NoError(err)
	s.Require().NoError(spec.Validate(loader.Context))
	router, err := gorillamux.NewRouter(spec)
	s.Require().NoError(err)
	s.router = router
}

func (s *APISuite) TearDownSuite() {
	fmt.Println("teardown")
	s.server.Close()
}

func (s *APISuite) TestRoot() {
	resp := s.get("/")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	// the version is build-time injected
	s.Require().Contains(string(body), "apex version")
}

func (s *APISuite) TestInvalidPath() {
	resp := s.get("/invalid-path")
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

// Spec validation

// checks that resp satisfies the Open API spec
// see https://github.com/getkin/kin-openapi#validating-http-requestsresponses
func (s *APISuite) validateAgainstSpec(resp *http.Response) {
	route, pathParams, err := s.router.FindRoute(resp.Request)
	if err != nil {
		return // path not in spec - not a error
	}

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body)) // put body back for main tests

	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    resp.Request,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  resp.StatusCode,
		Header:  resp.Header,
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	in.SetBodyBytes(body)

	err = openapi3filter.ValidateResponse(context.Background(), in)
	s.Require().NoError(err,
		"%s %s: Open API validation failed: response does not satisfy api.yaml",
		resp.Request.Method,
		resp.Request.URL.Path,
	)
}

// Utils

func (s *APISuite) get(path string) *http.Response {
	resp, err := s.client.Get(s.server.URL + path)
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	return resp
}

// a bodyless POST (close/open style endpoints)
func (s *APISuite) post(path string) *http.Response {
	resp, err := s.client.Post(s.server.URL+path, "", nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	return resp
}

func (s *APISuite) postJSON(path string, payload any) *http.Response {
	body, err := json.Marshal(payload)
	s.Require().NoError(err)
	resp, err := s.client.Post(s.server.URL+path, "application/json", bytes.NewReader(body))
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	return resp
}

func (s *APISuite) postJSONWithHeaders(path string, payload any, headers map[string]string) *http.Response {
	body, err := json.Marshal(payload)
	s.Require().NoError(err)
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, bytes.NewReader(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	return resp
}

func (s *APISuite) putJSON(path string, payload any) *http.Response {
	body, err := json.Marshal(payload)
	s.Require().NoError(err)
	req, err := http.NewRequest(http.MethodPut, s.server.URL+path, bytes.NewReader(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	return resp
}

func (s *APISuite) decodeJSON(resp *http.Response, target any) {
	err := json.NewDecoder(resp.Body).Decode(target)
	s.Require().NoError(err)
}

// Mocked storage

func getMockedStorage() storage.Storage {
	return &mockStorage{}
}

type mockStorage struct {
	pingErr error
}

func (ms *mockStorage) Ping(context.Context) error {
	return ms.pingErr
}

func (ms *mockStorage) CreatePlayerProfile(c context.Context, profile *player.Profile, idempotencyKey string) (player.ID, error) {
	fmt.Printf("creating profile %v to mocked storage", profile)
	if idempotencyKey == MockedConflictIdempotencyKey {
		return "", storage.ErrIdempotencyConflict
	}
	return profile.PlayerId, nil
}

func (ms *mockStorage) GetPlayerProfile(c context.Context, id player.ID) (*player.Profile, error) {
	profile := player.Profile{
		PlayerId:   player.ID(MockedPlayerId),
		PlayerName: "Mighty Warrior",
	}
	fmt.Printf("getting stabbed profile %v from mocked storage", profile)
	return &profile, nil
}

func (ms *mockStorage) CreateBoard(
	ctx context.Context,
	board *board.Board,
) error {
	fmt.Printf("creating board %v to mocked storage", board)
	return nil
}

func (ms *mockStorage) SetBoardState(c context.Context, boardId board.ID, state board.BoardState) error {
	fmt.Printf("setting board %v state %v in mocked storage", boardId, state)
	if boardId == board.ID(MockedUnknownBoardId) {
		return storage.ErrBoardNotFound
	}
	return nil
}

func (ms *mockStorage) GetBoard(c context.Context, boardId board.ID) (*board.Board, error) {
	return &board.Board{
		BoardId:   boardId,
		BoardName: "Mocked Board",
		State:     board.BoardActive,
		CreatedAt: time.UnixMilli(100),
	}, nil
}

func (ms *mockStorage) ListBoards(c context.Context) ([]board.Board, error) {
	return []board.Board{
		{BoardId: board.MainId, BoardName: "main", State: board.BoardActive, CreatedAt: time.UnixMilli(100)},
		{BoardId: "summer-contest", BoardName: "Summer Contest", State: board.BoardClosed, CreatedAt: time.UnixMilli(200)},
	}, nil
}

func (ms *mockStorage) IncrementScore(c context.Context, playerId player.ID, boardId board.ID, amount float64, requestID, idempotencyKey string) error {
	if boardId == board.ID(MockedClosedBoardId) {
		return storage.ErrBoardClosed
	}
	return nil
}

func (ms *mockStorage) SetScore(c context.Context, playerId player.ID, boardId board.ID, score float64, requestID, idempotencyKey string) error {
	if boardId == board.ID(MockedClosedBoardId) {
		return storage.ErrBoardClosed
	}
	return nil
}

func (ms *mockStorage) GetStanding(c context.Context, playerId player.ID, boardId board.ID) (storage.Standing, int64, error) {
	return storage.Standing{PlayerID: string(playerId), Score: 46.4, Rank: 3}, 10, nil
}

func (ms *mockStorage) ListStandings(c context.Context, boardId board.ID, limit, offset int64) ([]storage.Standing, int64, error) {
	return []storage.Standing{
		{PlayerID: MockedPlayerId, Score: 46.4, Rank: 1},
		{PlayerID: "0f8fad5b-d9cb-469f-a165-70867728950e", Score: 30.0, Rank: 2},
	}, 2, nil
}

func (ms *mockStorage) PlayerHistory(c context.Context, playerId player.ID, boardId board.ID, limit int64) ([]ledger.Event, error) {
	return []ledger.Event{
		{ID: "200-1", Type: ledger.EventIncrement, PlayerID: string(playerId), BoardID: string(boardId), Amount: 3, RequestID: "r2", CreatedAt: time.UnixMilli(200)},
		{ID: "100-0", Type: ledger.EventSet, PlayerID: string(playerId), BoardID: string(boardId), Amount: 0, RequestID: "r1", CreatedAt: time.UnixMilli(100)},
	}, nil
}

func (ms *mockStorage) ListEventsAfter(
	_ context.Context, after string, limit int64,
) ([]ledger.Event, error) {
	// TODO: mocked methods with such complex logic is hard to maintain, consider rewriting
	if after == "999-0" {
		return []ledger.Event{}, nil
	}

	start, _, _ := strings.Cut(after, "-")
	startMillis, _ := strconv.ParseInt(start, 10, 64)
	events := make([]ledger.Event, 0, limit)
	for i := int64(1); i <= limit; i++ {
		milliseconds := startMillis + i
		events = append(events, ledger.Event{
			ID:        fmt.Sprintf("%d-0", milliseconds),
			Type:      ledger.EventIncrement,
			PlayerID:  MockedPlayerId,
			BoardID:   MockedBoardId,
			Amount:    5,
			RequestID: fmt.Sprintf("request-%d", i),
			CreatedAt: time.UnixMilli(milliseconds).UTC(),
		})
	}
	return events, nil
}

func (ms *mockStorage) RebuildProjection(c context.Context, boardId board.ID) error {
	if boardId == board.ID(MockedUnknownBoardId) {
		return storage.ErrBoardNotFound
	}
	return nil
}

func (ms *mockStorage) VerifyProjection(c context.Context, boardId board.ID) ([]storage.ScoreMismatch, error) {
	if boardId == board.ID(MockedUnknownBoardId) {
		return nil, storage.ErrBoardNotFound
	}
	return []storage.ScoreMismatch{
		{
			BoardID:       string(boardId),
			PlayerID:      MockedPlayerId,
			LiveScore:     40,
			LivePresent:   true,
			ReplayScore:   42,
			ReplayPresent: true,
		},
	}, nil
}

func (ms *mockStorage) ListDailyActivity(
	context.Context, string, int64,
) ([]storage.ActivityEntry, error) {
	return []storage.ActivityEntry{
		{PlayerID: MockedPlayerId, Count: 5},
		{PlayerID: "0f8fad5b-d9cb-469f-a165-70867728950e", Count: 3},
	}, nil
}
