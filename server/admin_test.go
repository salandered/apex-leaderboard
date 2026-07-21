package server_test

import (
	"net/http"

	"github.com/salandered/apex/handlers"
)

func (s *APISuite) TestAdminRebuildProjection() {
	resp := s.post("/api/v1/admin/boards/" + MockedBoardId + "/projection/rebuild")
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
}

func (s *APISuite) TestAdminRebuildProjectionUnknownBoard() {
	resp := s.post("/api/v1/admin/boards/" + MockedUnknownBoardId + "/projection/rebuild")
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestAdminRebuildProjectionInvalidBoardId() {
	resp := s.post("/api/v1/admin/boards/Bad_Id/projection/rebuild")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestAdminVerifyProjection() {
	resp := s.get("/api/v1/admin/boards/" + MockedBoardId + "/projection/verify")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.VerifyProjectionResp
	s.decodeJSON(resp, &result)
	s.Require().Len(result.Mismatches, 1)
	s.Require().Equal(MockedBoardId, result.Mismatches[0].BoardId)
	s.Require().Equal(MockedPlayerId, result.Mismatches[0].PlayerId)
	s.Require().Equal(40.0, result.Mismatches[0].LiveScore)
	s.Require().Equal(42.0, result.Mismatches[0].ReplayScore)
}
