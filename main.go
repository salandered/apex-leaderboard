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

func getMux(s storage.Storage) *http.ServeMux {
	handler := &handlers.HTTPHandler{
		Storage: s,
	}

	mux := http.NewServeMux()
	// using snaked 'player_id' naming to match the OpenAPI spec
	mux.HandleFunc("GET /{$}", handler.HandleRoot)
	mux.HandleFunc("POST /api/v1/scores", handler.HandlePostPlayer)
	mux.HandleFunc("GET /api/v1/scores/{player_id}", handler.HandleGetScore)
	mux.HandleFunc("GET /api/v1/scores/{player_id}/history", handler.HandleGetHistory)
	mux.HandleFunc("PUT /api/v1/scores/{player_id}", handler.HandleSetScore)
	mux.HandleFunc("POST /api/v1/scores/{player_id}/increment", handler.HandleIncrementScore)

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
	// TODO: get range of users (pagination)

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
}
