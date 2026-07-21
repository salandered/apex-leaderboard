package server_test

import (
	"net/http"

	"github.com/salandered/apex/handlers"
)

func (s *APISuite) TestListEvents() {
	resp := s.get("/api/v1/events?after=0-0&limit=2")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListEventsResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Events, 2)
	s.Require().Equal("2-0", result.NextAfter)
	s.Require().Equal(handlers.ScoreEvent{
		EventId:   "1-0",
		Type:      "increment",
		PlayerId:  MockedPlayerId,
		BoardId:   MockedBoardId,
		Amount:    5,
		RequestId: "request-1",
		CreatedAt: "1970-01-01T00:00:00.001Z",
	}, result.Events[0])
}

func (s *APISuite) TestListEventsEmptyRetainsCursor() {
	resp := s.get("/api/v1/events?after=999-0&limit=2")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListEventsResp
	s.decodeJSON(resp, &result)
	s.Require().Empty(result.Events)
	s.Require().Equal("999-0", result.NextAfter)
}

func (s *APISuite) TestListEventsDefaultLimit() {
	resp := s.get("/api/v1/events?after=0-0")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListEventsResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Events, 50)
	s.Require().Equal("50-0", result.NextAfter)
}

func (s *APISuite) TestListEventsMaximumLimit() {
	resp := s.get("/api/v1/events?after=0-0&limit=100")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListEventsResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Events, 100)
	s.Require().Equal("100-0", result.NextAfter)
}

func (s *APISuite) TestListEventsMissingCursor() {
	resp := s.get("/api/v1/events")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListEventsInvalidCursor() {
	resp := s.get("/api/v1/events?after=invalid")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListEventsInvalidCursorMilliseconds() {
	resp := s.get("/api/v1/events?after=nope-0")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListEventsInvalidCursorSequence() {
	resp := s.get("/api/v1/events?after=1-nope")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListEventsLimitTooSmall() {
	resp := s.get("/api/v1/events?after=0-0&limit=0")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListEventsLimitTooLarge() {
	resp := s.get("/api/v1/events?after=0-0&limit=101")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}
