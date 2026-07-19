package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
	"github.com/stretchr/testify/suite"
)

var MockedPlayerId = "698057b7-eb86-4f63-a228-100304c6ca0a"
var MockedBoardId = "main"
var MockedUnknownBoardId = "ghost-board" // the mock answers ErrBoardNotFound
var MockedClosedBoardId = "closed-board" // the mock rejects score writes with ErrBoardClosed

//go:embed api.yaml
var apiSpec []byte

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
	s.server = httptest.NewServer(getMux(getMockedStorage()))
	s.client = s.server.Client()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(apiSpec)
	s.Require().NoError(err)
	s.Require().NoError(doc.Validate(loader.Context))
	router, err := gorillamux.NewRouter(doc)
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
	s.Require().Contains(string(body), "apex")
	s.Require().Contains(string(body), "see /api/v1/scores")
}

func (s *APISuite) TestInvalidPath() {
	resp := s.get("/invalid-path")
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestPostPlayer() {
	resp := s.postJSON("/api/v1/players", handlers.PostPlayerReq{
		PlayerName: "alice",
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var result handlers.PostPlayerResp
	s.decodeJSON(resp, &result)
	s.Require().NotEmpty(result.PlayerId)
	s.Require().NoError(player.ID(result.PlayerId).Validate())
}

func (s *APISuite) TestPutScore() {
	resp := s.putJSON(
		"/api/v1/boards/"+MockedBoardId+"/scores/"+MockedPlayerId,
		handlers.PutScoreReq{
			PlayerScore: 98.0,
		})
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestIncrementScore() {
	resp := s.postJSON(
		"/api/v1/boards/"+MockedBoardId+"/scores/"+MockedPlayerId+"/increment",
		handlers.IncrementScoreReq{
			Amount: 12.0,
		})

	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.IncrementScoreResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(12.0, result.Score)
}

func (s *APISuite) TestPutBoard() {
	resp := s.putJSON("/api/v1/boards/summer-contest", handlers.PutBoardReq{
		BoardName: "Summer Contest",
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)
}

func (s *APISuite) TestPutBoardInvalidId() {
	resp := s.putJSON("/api/v1/boards/Bad_Id", handlers.PutBoardReq{
		BoardName: "nope",
	})
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetBoard() {
	resp := s.get("/api/v1/boards/" + MockedBoardId)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.BoardResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedBoardId, result.BoardId)
	s.Require().NotEmpty(result.BoardName)
	s.Require().Equal("active", result.State)
	s.Require().NotEmpty(result.CreatedAt)
}

func (s *APISuite) TestListBoards() {
	resp := s.get("/api/v1/boards")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListBoardsResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Boards, 2)
	s.Require().Equal("main", result.Boards[0].BoardId) // creation order
	s.Require().Equal("active", result.Boards[0].State)
	s.Require().Equal("closed", result.Boards[1].State)
}

func (s *APISuite) TestCloseBoard() {
	resp := s.post("/api/v1/boards/" + MockedBoardId + "/close")
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestOpenBoard() {
	resp := s.post("/api/v1/boards/" + MockedBoardId + "/open")
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestCloseBoardUnknownId() {
	resp := s.post("/api/v1/boards/" + MockedUnknownBoardId + "/close")
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestCloseBoardInvalidId() {
	resp := s.post("/api/v1/boards/Bad_Id/close")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestPutScoreClosedBoard() {
	resp := s.putJSON(
		"/api/v1/boards/"+MockedClosedBoardId+"/scores/"+MockedPlayerId,
		handlers.PutScoreReq{
			PlayerScore: 98.0,
		})
	s.Require().Equal(http.StatusConflict, resp.StatusCode)
}

func (s *APISuite) TestIncrementScoreClosedBoard() {
	resp := s.postJSON(
		"/api/v1/boards/"+MockedClosedBoardId+"/scores/"+MockedPlayerId+"/increment",
		handlers.IncrementScoreReq{
			Amount: 12.0,
		})
	s.Require().Equal(http.StatusConflict, resp.StatusCode)
}

func (s *APISuite) TestListScoresOnBoard() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?limit=10")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListScoresResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Scores, 2)
}

func (s *APISuite) TestGetStandingOnBoard() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/" + MockedPlayerId)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.RankResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
	s.Require().Equal(int64(3), result.Rank)
}

func (s *APISuite) TestGetHistoryOnBoard() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/" + MockedPlayerId + "/history")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.HistoryResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Events, 2)
}

func (s *APISuite) TestGetScore() {
	resp := s.get("/api/v1/players/" + MockedPlayerId)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.GetPlayerResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
}

func (s *APISuite) TestGetScoreInvalidId() {
	resp := s.get("/api/v1/players/not-a-uuid")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetRank() {
	resp := s.get("/api/v1/scores/" + MockedPlayerId + "/rank")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.RankResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
	s.Require().Equal(int64(3), result.Rank)
	s.Require().Equal(int64(10), result.Total)
}

func (s *APISuite) TestGetRankInvalidId() {
	resp := s.get("/api/v1/scores/not-a-uuid/rank")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScores() {
	resp := s.get("/api/v1/scores?limit=10&offset=0")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListScoresResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Scores, 2)
	s.Require().Equal(MockedPlayerId, result.Scores[0].PlayerId)
	s.Require().Equal(int64(1), result.Scores[0].Rank) // highest first
	s.Require().Equal(46.4, result.Scores[0].Score)
	s.Require().Equal(int64(2), result.Scores[1].Rank)
	s.Require().Equal(30.0, result.Scores[1].Score)

	s.Require().Equal(int64(2), result.Total)
}

func (s *APISuite) TestListScoresInvalidOffset() {
	resp := s.get("/api/v1/scores?offset=-1")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresLimitTooLarge() {
	resp := s.get("/api/v1/scores?limit=99999")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetHistory() {
	resp := s.get("/api/v1/scores/" + MockedPlayerId + "/history")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.HistoryResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
	s.Require().Len(result.Events, 2)
	s.Require().Equal("increment", result.Events[0].Type) // newest first
	s.Require().Equal("set", result.Events[1].Type)
}

func (s *APISuite) TestGetHistoryInvalidId() {
	resp := s.get("/api/v1/scores/not-a-uuid/history")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetHistoryInvalidLimit() {
	resp := s.get("/api/v1/scores/" + MockedPlayerId + "/history?limit=0")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
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

func getMockedStorage() apiStorage {
	return &mockStorage{}
}

type mockStorage struct {
}

func (ms *mockStorage) CreatePlayerProfile(c context.Context, profile *player.Profile, requestID string) error {
	fmt.Printf("creating profile %v to mocked storage", profile)
	return nil
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
	requestID string,
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

func (ms *mockStorage) IncrementScore(c context.Context, playerId player.ID, boardId board.ID, amount float64, requestID string) (float64, error) {
	if boardId == board.ID(MockedClosedBoardId) {
		return 0, storage.ErrBoardClosed
	}
	return 12.0, nil
}

func (ms *mockStorage) SetScore(c context.Context, playerId player.ID, boardId board.ID, score float64, requestID string) error {
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
		{ID: "200-1", Type: ledger.EventIncrement, PlayerID: string(playerId), Amount: 3, RequestID: "r2", CreatedAt: time.UnixMilli(200)},
		{ID: "100-0", Type: ledger.EventSet, PlayerID: string(playerId), Amount: 0, RequestID: "r1", CreatedAt: time.UnixMilli(100)},
	}, nil
}
