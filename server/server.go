package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
)

const addr = ":8090"

func NewMux(s storage.Storage) *http.ServeMux {
	health := &handlers.HealthHandler{Storage: s}
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

// Start runs the server until ctx is cancelled, then drains in-flight requests
// within a bounded window. Returns nil on a clean shutdown, or a non-nil error
// on a listener failure or a drain-deadline miss.
func Start(ctx context.Context, handler http.Handler) error {
	srv := &http.Server{
		Addr:           addr,
		Handler:        loggingMiddleware(handler),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
	}
	slog.Info("starting server", "addr", srv.Addr)

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		slog.Error("server error", "err", err)
		return err // ListenAndServe returned unexpected error -> fail fast
	case <-ctx.Done(): // signal -> graceful shutdown
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Shutdown(shutdownCtx) // when succeeds -> ListenAndServe returns ErrServerClosed
	slog.Info("shutdown", "err", err)
	return err
}
