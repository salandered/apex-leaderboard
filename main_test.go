package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
	"github.com/stretchr/testify/suite"
)

func TestAPI(t *testing.T) {
	suite.Run(t, new(APISuite))
}

type APISuite struct {
	suite.Suite
	server *httptest.Server
	client *http.Client
}

func (s *APISuite) SetupSuite() {
	fmt.Println("SetupSuite")
	s.server = httptest.NewServer(getMux(storage.NewStorage()))
	s.client = s.server.Client()
}

func (s *APISuite) TearDownSuite() {
	fmt.Println("teardown")
	s.server.Close()
}

func (s *APISuite) TestRoot() {
	resp, err := s.client.Get(s.server.URL + "/")

	s.Require().NoError(err)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Require().Equal("Root handled\n", string(body))
}

func (s *APISuite) TestNotFound() {
	resp, err := s.client.Get(s.server.URL + "/invalid-path")

	s.Require().NoError(err)
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestPostScore() {
	body, err := json.Marshal(handlers.PostRequestData{
		PlayerName:  "alice",
		PlayerScore: 42.5,
	})
	s.Require().NoError(err)

	resp, err := s.client.Post(s.server.URL+"/api/scores", "application/json", bytes.NewReader(body))
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var result handlers.PostResponseData
	err = json.NewDecoder(resp.Body).Decode(&result)
	s.Require().NoError(err)
	s.Require().NotEmpty(result.Id)
}
