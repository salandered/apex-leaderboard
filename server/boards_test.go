package server_test

import (
	"net/http"

	"github.com/salandered/apex/handlers"
)

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

func (s *APISuite) TestPutBoardCreatedClosed() {
	resp := s.putJSON("/api/v1/boards/summer-contest", handlers.PutBoardReq{
		BoardName: "Summer Contest",
		State:     "closed",
	})
	s.Require().Equal(http.StatusCreated, resp.StatusCode)
}

func (s *APISuite) TestPutBoardUnknownState() {
	resp := s.putJSON("/api/v1/boards/summer-contest", handlers.PutBoardReq{
		BoardName: "Summer Contest",
		State:     "paused",
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
