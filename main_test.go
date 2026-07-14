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

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/models"
	playerid "github.com/salandered/apex/player_id"
	"github.com/salandered/apex/storage"
	"github.com/stretchr/testify/suite"
)

var MockedUUID = "698057b7-eb86-4f63-a228-100304c6ca0a"

//go:embed api.yaml
var apiSpec []byte

func TestAPI(t *testing.T) {
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

	s.Require().Equal("Root handled\n", string(body))
}

func (s *APISuite) TestGetScore() {
	resp := s.get("/api/v1/scores/" + MockedUUID)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.GetResponseData
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedUUID, string(result.PlayerId))
}

func (s *APISuite) TestGetScoreInvalidId() {
	resp := s.get("/api/v1/scores/not-a-uuid")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestNotFound() {
	resp := s.get("/invalid-path")
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *APISuite) TestPostScore() {
	resp := s.postJSON("/api/v1/scores", handlers.PostRequestData{
		PlayerName:  "alice",
		PlayerScore: 42.5,
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var result handlers.PostResponseData
	s.decodeJSON(resp, &result)
	s.Require().NotEmpty(result.PlayerId)
}

func (s *APISuite) TestIncrementScore() {
	resp := s.postJSON("/api/v1/scores/"+MockedUUID+"/increment", handlers.IncrementScoreRequest{
		Amount: 12.0,
	})

	// then/
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.IncrementScoreResponse
	s.decodeJSON(resp, &result)
	s.Require().Equal(12.0, result.Score)
}

func (s *APISuite) get(path string) *http.Response {
	resp, err := s.client.Get(s.server.URL + path)
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

func (ms *mockStorage) CreatePlayer(c context.Context, profile *models.Profile, score float64) error {
	fmt.Printf("putting profile %v (score %v) to mocked storage", profile, score)
	return nil
}

func (ms *mockStorage) GetPlayer(c context.Context, id playerid.PlayerId) (*models.Profile, float64, error) {
	profile := models.Profile{
		PlayerId:   playerid.PlayerId(MockedUUID),
		PlayerName: "Mighty Warrior",
	}
	fmt.Printf("getting stabbed profile %v from mocked storage", profile)
	return &profile, 46.4, nil
}

func (ms *mockStorage) IncrementScore(c context.Context, playerId playerid.PlayerId, amount float64) (float64, error) {
	return 12.0, nil
}
