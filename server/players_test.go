package server_test

import (
	"net/http"
	"strings"

	"github.com/salandered/apex/handlers"
)

func (s *APISuite) TestPostPlayer() {
	resp := s.postJSON("/api/v1/players", handlers.PostPlayerReq{
		PlayerName: "alice",
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var result handlers.PostPlayerResp
	s.decodeJSON(resp, &result)
	s.Require().NotEmpty(result.Player.PlayerId)
	s.Require().NoError(result.Player.PlayerId.Validate())
	s.Require().Equal("alice", result.Player.PlayerName)
	s.Require().Equal("/api/v1/players/"+string(result.Player.PlayerId), resp.Header.Get("Location"))
}

func (s *APISuite) TestPostPlayerStoresTrimmedName() {
	resp := s.postJSON("/api/v1/players", handlers.PostPlayerReq{
		PlayerName: "  Mighty Warrior  ",
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)
	s.Require().Equal("Mighty Warrior", s.storage.lastProfile.PlayerName)
}

func (s *APISuite) TestPostPlayerRejectsInvalidName() {
	resp := s.postJSON("/api/v1/players", handlers.PostPlayerReq{PlayerName: ""})
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestPostPlayerWithIdempotencyKey() {
	resp := s.postJSONWithHeaders("/api/v1/players",
		handlers.PostPlayerReq{PlayerName: "alice"},
		map[string]string{"Idempotency-Key": "player-key-1"},
	)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)
}

func (s *APISuite) TestPostPlayerRejectsBigIdempotencyKey() {
	resp := s.postJSONWithHeaders("/api/v1/players",
		handlers.PostPlayerReq{PlayerName: "alice"},
		map[string]string{"Idempotency-Key": strings.Repeat("k", 129)},
	)
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *APISuite) TestPostPlayerIdempotencyKeyConflict() {
	resp := s.postJSONWithHeaders("/api/v1/players",
		handlers.PostPlayerReq{PlayerName: "alice"},
		map[string]string{"Idempotency-Key": MockedConflictIdempotencyKey},
	)
	s.Require().Equal(http.StatusConflict, resp.StatusCode)
}

func (s *APISuite) TestGetScore() {
	resp := s.get("/api/v1/players/" + MockedPlayerId)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var result handlers.GetPlayerResp
	s.decodeJSON(resp, &result)
	s.Require().Equal(MockedPlayerId, string(result.Player.PlayerId))
}

func (s *APISuite) TestGetScoreInvalidId() {
	resp := s.get("/api/v1/players/not-a-uuid")
	s.Require().Equal(http.StatusBadRequest, resp.StatusCode)
}
