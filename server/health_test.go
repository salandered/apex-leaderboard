package server_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/server"
)

func (s *APISuite) TestLivezReturnsOK() {
	resp := s.get("/livez")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.HealthResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(handlers.HealthResp{Status: "ok"}, result)
}

func (s *APISuite) TestReadyzReturnsOK() {
	resp := s.get("/readyz")
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.HealthResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(handlers.HealthResp{Status: "ok"}, result)
}

func (s *APISuite) TestReadyWhenStorageIsUnavailable() {
	testServer := httptest.NewServer(server.NewMux(&mockStorage{
		pingErr: errors.New("redis unavailable"),
	}, func() bool { return true }))
	defer testServer.Close()

	resp, err := testServer.Client().Get(testServer.URL + "/readyz")
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	s.Require().Equal(http.StatusServiceUnavailable, resp.StatusCode)

	var result handlers.HealthResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(handlers.HealthResp{Status: "unavailable", Dependency: "redis"}, result)
}

func (s *APISuite) TestReadyWhenMainBoardNotSeeded() {
	testServer := httptest.NewServer(server.NewMux(&mockStorage{}, func() bool { return false }))
	defer testServer.Close()

	resp, err := testServer.Client().Get(testServer.URL + "/readyz")
	s.Require().NoError(err)
	s.T().Cleanup(func() { resp.Body.Close() })
	s.validateAgainstSpec(resp)
	s.Require().Equal(http.StatusServiceUnavailable, resp.StatusCode)

	var result handlers.HealthResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(handlers.HealthResp{Status: "unavailable", Dependency: "seed"}, result)
}
