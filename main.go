package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

// storage.ProjectionAdmin is absent - admin ops (replay/verify) are not
// reachable from these routes; a future admin surface declares it separately.
type apiStorage interface {
	storage.PlayerRepo
	storage.BoardRepo
	storage.ScoreRepo
}

func getMux(s apiStorage) *http.ServeMux {
	players := &handlers.PlayerHandler{Storage: s}
	boards := &handlers.BoardHandler{Storage: s}
	scores := &handlers.ScoreHandler{Storage: s}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", handlers.HandleRoot)
	// players
	mux.HandleFunc("POST /api/v1/players", players.HandlePostPlayer)
	// profile read lives under /scores until the players namespace lands
	mux.HandleFunc("GET /api/v1/scores/{player_id}", players.HandleGetPlayer)
	// boards
	mux.HandleFunc("PUT /api/v1/boards/{board_id}", boards.HandlePutBoard)
	// mux.HandleFunc get boards
	// scores
	mux.HandleFunc("PUT /api/v1/boards/{board_id}/scores/{player_id}", scores.HandlePutScore)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/scores/{player_id}/increment", scores.HandleIncrementScore)
	mux.HandleFunc("GET /api/v1/scores/{player_id}/rank", scores.HandleGetRank)
	mux.HandleFunc("GET /api/v1/scores", scores.HandleListScores)
	// ledger
	mux.HandleFunc("GET /api/v1/scores/{player_id}/history", scores.HandleGetHistory)

	return mux
}

func startServer(handler http.Handler) {
	s := &http.Server{
		Addr:           ":8090",
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
	}
	slog.Info("starting server", "addr", s.Addr)
	slog.Error("server stopped", "error", s.ListenAndServe())
	os.Exit(1)
}

func main() {
	logCloser, err := logging.Setup()
	if err != nil {
		// logger isn't ready yet, report to stderr directly
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logCloser.Close()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = defaultRedisURL
	}

	store, err := storage.NewStorage(redisURL)
	if err != nil {
		slog.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	mux := getMux(store)
	startServer(loggingMiddleware(mux))

	// TODO bootstrap: add main board
}
