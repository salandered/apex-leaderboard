package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

const banner = `
       _________        _________        _________        _________
      /    A    /\     /    P    /\     /    E    /\     /    X    /\
     /_________/  \___/_________/  \___/_________/  \___/_________/  \
     \         \  /   \         \  /   \         \  /   \         \  /
      \_________\/     \_________\/     \_________\/     \_________\/`

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
	mux.HandleFunc("GET /api/v1/players/{player_id}", players.HandleGetPlayer)
	// boards
	mux.HandleFunc("PUT /api/v1/boards/{board_id}", boards.HandlePutBoard)
	mux.HandleFunc("GET /api/v1/boards", boards.HandleListBoards)
	mux.HandleFunc("GET /api/v1/boards/{board_id}", boards.HandleGetBoard)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/close", boards.HandleCloseBoard)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/open", boards.HandleOpenBoard)
	// scores + ledger, board-scoped
	mux.HandleFunc("PUT /api/v1/boards/{board_id}/scores/{player_id}", scores.HandlePutScore)
	mux.HandleFunc("POST /api/v1/boards/{board_id}/scores/{player_id}/increment", scores.HandleIncrementScore)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores", scores.HandleListScores)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores/{player_id}", scores.HandleGetRank)
	mux.HandleFunc("GET /api/v1/boards/{board_id}/scores/{player_id}/history", scores.HandleGetHistory)
	// legacy aliases for the main board (documented as such in api.yaml)
	mux.HandleFunc("GET /api/v1/scores", scores.HandleListScores)
	mux.HandleFunc("GET /api/v1/scores/{player_id}/rank", scores.HandleGetRank)
	mux.HandleFunc("GET /api/v1/scores/{player_id}/history", scores.HandleGetHistory)

	return mux
}

// creates the default board if missing
func createMainBoard(s storage.BoardRepo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.CreateBoard(ctx, &board.Board{
		BoardId:   board.MainId,
		BoardName: "main",
		State:     board.BoardActive,
		CreatedAt: apextime.Now(),
	}, "seed-main")
	if errors.Is(err, storage.ErrBoardExists) {
		return nil
	}
	return err
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
	fmt.Printf("apex version %v %v \n\n", handlers.GetVersion(), banner)

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

	// the default board must exist before the server accepts writes
	if err := createMainBoard(store); err != nil {
		slog.Error("seeding main board failed", "error", err)
		os.Exit(1)
	}

	mux := getMux(store)
	startServer(loggingMiddleware(mux))
}
