package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/models"
	playerid "github.com/salandered/apex/player_id"
	"github.com/salandered/apex/storage"
	"github.com/stretchr/testify/suite"
)

var MockedUUID = "698057b7-eb86-4f63-a228-100304c6ca0a"

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
	s.server = httptest.NewServer(getMux(getMockedStorage()))
	s.client = s.server.Client()
}

func (s *APISuite) TearDownSuite() {
	fmt.Println("teardown")
	s.server.Close()
}

func (s *APISuite) TestRoot() {
	resp, err := s.client.Get(s.server.URL + "/")

	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	s.Require().Equal("Root handled\n", string(body))
}

func (s *APISuite) TestGetScore() {
	resp, err := s.client.Get(s.server.URL + "/api/scores/" + MockedUUID)
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.GetResponseData
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedUUID, string(result.PlayerId))
}

func (s *APISuite) TestGetScoreInvalidId() {
	resp, err := s.client.Get(s.server.URL + "/api/scores/not-a-uuid")
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestNotFound() {
	resp, err := s.client.Get(s.server.URL + "/invalid-path")

	s.Require().NoError(err)
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestPostScore() {
	resp := s.postJSON("/api/scores", handlers.PostRequestData{
		PlayerName:  "alice",
		PlayerScore: 42.5,
	})
	defer resp.Body.Close()
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var result handlers.PostResponseData
	s.decodeJSON(resp, &result)
	s.Require().NotEmpty(result.PlayerId)
}

func (s *APISuite) postJSON(path string, payload any) *http.Response {
	body, err := json.Marshal(payload)
	s.Require().NoError(err)
	resp, err := s.client.Post(s.server.URL+path, "application/json", bytes.NewReader(body))
	s.Require().NoError(err)
	return resp
}

func (s *APISuite) decodeJSON(resp *http.Response, target any) {
	err := json.NewDecoder(resp.Body).Decode(target)
	s.Require().NoError(err)
}

func getMockedStorage() storage.Storage {
	// storage.NewStorage()
	return &mockStorage{}
}

type mockStorage struct {
}

func (ms *mockStorage) PutData(c context.Context, playerData *models.PlayerData) error {
	fmt.Printf("putting data %v to mocked storage", playerData)
	return nil
}

func (ms *mockStorage) GetData(c context.Context, id playerid.PlayerId) (*models.PlayerData, error) {
	playerData := models.PlayerData{
		PlayerId:    playerid.PlayerId(MockedUUID),
		PlayerName:  "Mighty Warrior",
		PlayerScore: 46.4,
	}
	fmt.Printf("getting stabbed data %v from mocked storage", playerData)
	return &playerData, nil
}
