package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
)

const addr = ":8090"

const DefaultShutdownTimeout = 10 * time.Second

var (
	ServerError = errors.New("server")
	ErrServe    = fmt.Errorf("%w: serve failed", ServerError)
	ErrShutdown = fmt.Errorf("%w: shutdown incomplete", ServerError)
)

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

// Start runs the server until ctx is cancelled, then shuts down in-flight requests.
// The caller owns error logging.
func Start(ctx context.Context, handler http.Handler, shutdownTimeout time.Duration) error {
	srv := &http.Server{
		Addr:           addr,
		Handler:        loggingMiddleware(handler),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
	}
	slog.Info("starting server", "addr", srv.Addr)

	errServeCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errServeCh <- err
		}
	}()

	select {
	case err := <-errServeCh:
		return fmt.Errorf("%w: %w", ErrServe, err) // ListenAndServe returned unexpected error -> fail fast
	case <-ctx.Done(): // signal -> graceful shutdown
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil { // when succeeds -> ListenAndServe returns ErrServerClosed
		return fmt.Errorf("%w: %w", ErrShutdown, err)
	}

	// in case errServeCh holds something (first select might ve chosen ctx.Done)
	select {
	case err := <-errServeCh:
		return fmt.Errorf("%w: %w", ErrServe, err)
	default:
	}

	slog.Info("server stopped")
	return nil
}
