package handlers

import (
	"net/http"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/storage"
)

// BoardHandler serves the board endpoints.
type BoardHandler struct {
	Storage storage.BoardRepo
}

type PutBoardReq struct {
	BoardName string `json:"board_name"`
	State     string `json:"status,omitempty"` // state to status; empty means active
}

type BoardResp struct {
	BoardId   string `json:"board_id"`
	BoardName string `json:"board_name"`
	State     string `json:"status"` // state to status
	CreatedAt string `json:"created_at"`
}

type GetBoardResp struct {
	Board BoardResp `json:"board"`
}

type ListBoardsResp struct {
	Boards []BoardResp `json:"boards"`
}

func (h *BoardHandler) HandlePutBoard(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	var data PutBoardReq
	if err := readJSON(w, req, &data); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	newBoard, err := board.NewBoard(boardId, data.BoardName, board.BoardState(data.State))
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	if err := h.Storage.CreateBoard(req.Context(), newBoard); err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/boards/"+string(boardId))
	w.WriteHeader(http.StatusCreated)
}

func (h *BoardHandler) HandleGetBoard(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	b, err := h.Storage.GetBoard(req.Context(), boardId)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	writeJSONToResponse(req.Context(), w, http.StatusOK, GetBoardResp{Board: boardToResp(b)})
}

func (h *BoardHandler) HandleListBoards(w http.ResponseWriter, req *http.Request) {
	boards, err := h.Storage.ListBoards(req.Context())
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	response := ListBoardsResp{Boards: make([]BoardResp, 0, len(boards))}
	for i := range boards {
		response.Boards = append(response.Boards, boardToResp(&boards[i]))
	}

	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}

func (h *BoardHandler) HandleCloseBoard(w http.ResponseWriter, req *http.Request) {
	h.handleSetState(w, req, board.BoardClosed)
}

func (h *BoardHandler) HandleOpenBoard(w http.ResponseWriter, req *http.Request) {
	h.handleSetState(w, req, board.BoardActive)
}

func (h *BoardHandler) handleSetState(w http.ResponseWriter, req *http.Request, state board.BoardState) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	if err := h.Storage.SetBoardState(req.Context(), boardId, state); err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func boardToResp(b *board.Board) BoardResp {
	return BoardResp{
		BoardId:   string(b.BoardId),
		BoardName: b.BoardName,
		State:     string(b.State),
		CreatedAt: apextime.Format(b.CreatedAt),
	}
}
