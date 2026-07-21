package server_test

import (
	"net/http"

	"github.com/salandered/apex/handlers"
)

func (s *APISuite) TestListDailyActivity() {
	resp := s.get("/api/v1/activity/daily?date=2026-07-21&limit=10")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.ListDailyActivityResp
	s.decodeJSON(resp, &result)
	s.Require().Equal("2026-07-21", result.Date)
	s.Require().Len(result.Entries, 2)
	s.Require().Equal(MockedPlayerId, result.Entries[0].PlayerId)
	s.Require().Equal(int64(5), result.Entries[0].Count)
}

func (s *APISuite) TestListDailyActivityMissingDate() {
	resp := s.get("/api/v1/activity/daily?limit=10")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListDailyActivityInvalidDate() {
	resp := s.get("/api/v1/activity/daily?date=not-a-date")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestListDailyActivityLimitTooLarge() {
	resp := s.get("/api/v1/activity/daily?date=2026-07-21&limit=99999")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}
