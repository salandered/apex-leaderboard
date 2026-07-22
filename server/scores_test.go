package server_test

import (
	"net/http"
	"strings"

	"github.com/salandered/apex/handlers"
)

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

	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestIncrementScoreWithIdempotencyKey() {
	resp := s.postJSONWithHeaders(
		"/api/v1/boards/"+MockedBoardId+"/scores/"+MockedPlayerId+"/increment",
		handlers.IncrementScoreReq{Amount: 12.0},
		map[string]string{"Idempotency-Key": "key-abc"},
	)
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestIncrementScoreRejectsOverlongIdempotencyKey() {
	resp := s.postJSONWithHeaders(
		"/api/v1/boards/"+MockedBoardId+"/scores/"+MockedPlayerId+"/increment",
		handlers.IncrementScoreReq{Amount: 12.0},
		map[string]string{"Idempotency-Key": strings.Repeat("k", 129)},
	)
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

func (s *APISuite) TestListScoresOnBoardAsOf() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?as_of=2026-07-18")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListScoresResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(int64(1), result.Total)
	s.Require().Equal(12.5, result.Scores[0].Score)
}

func (s *APISuite) TestListScoresOnBoardRejectsInvalidAsOf() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?as_of=not-a-date")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresOnBoardRejectsEmptyAsOf() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?as_of=")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresOnBoardRejectsFutureAsOf() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?as_of=9999-01-01")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresUnknownBoardIsEmpty() {
	resp := s.get("/api/v1/boards/" + MockedUnknownBoardId + "/scores")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListScoresResp
	s.decodeJSON(resp, &result)
	s.Require().Empty(result.Scores)
	s.Require().Zero(result.Total)
}

func (s *APISuite) TestListScoresAsOfUnknownBoardIsEmpty() {
	resp := s.get("/api/v1/boards/" + MockedUnknownBoardId + "/scores?as_of=2026-07-18")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListScoresResp
	s.decodeJSON(resp, &result)
	s.Require().Empty(result.Scores)
	s.Require().Zero(result.Total)
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

func (s *APISuite) TestGetStandingOnBoardDetails() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/" + MockedPlayerId)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.RankResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
	s.Require().Equal(int64(3), result.Rank)
	s.Require().Equal(int64(10), result.Total)
}

func (s *APISuite) TestGetStandingOnBoardInvalidPlayerId() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/not-a-uuid")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresOnBoardDetails() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?limit=10&offset=0")
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

func (s *APISuite) TestListScoresOnBoardInvalidOffset() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?offset=-1")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListScoresOnBoardLimitTooLarge() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores?limit=99999")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetHistoryOnBoardDetails() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/" + MockedPlayerId + "/history")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.HistoryResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.PlayerId))
	s.Require().Len(result.Events, 2)
	s.Require().Equal(handlers.ScoreEvent{
		EventId:   "200-1",
		Type:      "increment",
		PlayerId:  MockedPlayerId,
		BoardId:   MockedBoardId,
		Amount:    3,
		RequestId: "r2",
		CreatedAt: "1970-01-01T00:00:00.200Z",
	}, result.Events[0]) // newest first
	s.Require().Equal("set", result.Events[1].Type)
}

func (s *APISuite) TestGetHistoryOnBoardInvalidPlayerId() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/not-a-uuid/history")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestGetHistoryOnBoardInvalidLimit() {
	resp := s.get("/api/v1/boards/" + MockedBoardId + "/scores/" + MockedPlayerId + "/history?limit=0")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}
