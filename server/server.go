package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
)

const addr = ":8090"

func NewMux(s storage.Storage, seeded func() bool) *http.ServeMux {
	health := &handlers.HealthHandler{Storage: s, Seeded: seeded}
	players := &handlers.PlayerHandler{Storage: s}
	boards := &handlers.BoardHandler{Storage: s}
	scores := &handlers.ScoreHandler{Storage: s}
	admin := &handlers.AdminHandler{Storage: s}
	views := &handlers.ViewHandler{Storage: s}
	events := &handlers.EventHandler{Storage: s}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", handlers.HandleRoot)
	mux.HandleFunc("GET /livez", health.HandleLive)
	mux.HandleFunc("GET /readyz", health.HandleReady)
	// players
	mux.HandleFunc("POST /api/v1/players", players.HandlePostPlayer)
	mux.HandleFunc("GET /api/v1/players/{player_id}", players.HandleGetPlayer)
	// boards
	mux.HandleFunc("PUT /api/v1/boards/{board_id}", boards.HandlePutBoard)
	mux.HandleFunc("GET /api/v1/boards", boards.HandleListBoards)
	mux.HandleFunc("GET /api/v1/boards/{board_id}", boards.HandleGetBoard)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/close", boards.HandleCloseBoard)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/open", boards.HandleOpenBoard)
	// scores, board-scoped
	mux.HandleFunc("PUT /api/v1/boards/{board_id}/scores/{player_id}", scores.HandlePutScore)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/scores/{player_id}/increment", scores.HandleIncrementScore)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores", scores.HandleListScores)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores/{player_id}", scores.HandleGetRank)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores/{player_id}/history", scores.HandleGetHistory)
	// global events
	mux.HandleFunc("GET /api/v1/events", events.HandleListEvents)

	// admin
	mux.HandleFunc(
		"POST /api/v1/admin/boards/{board_id}/projection/rebuild",
		admin.HandleRebuildProjection,
	)
	mux.HandleFunc(
		"GET /api/v1/admin/boards/{board_id}/projection/verify",
		admin.HandleVerifyProjection,
	)

	// async projections
	mux.HandleFunc("GET /api/v1/activity/daily", views.HandleListDailyActivity)

	return mux
}

// blocks until the server stops and always returns a non-nil error.
func Start(handler http.Handler) error {
	s := &http.Server{
		Addr:           addr,
		Handler:        loggingMiddleware(handler),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
	}
	slog.Info("starting server", "addr", s.Addr)
	return s.ListenAndServe()
}
