package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/server"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

var ErrConfig = errors.New("invalid config")

const workerStopMargin = 2 * time.Second

const banner = `
       _________        _________        _________        _________
      /    A    /\     /    P    /\     /    E    /\     /    X    /\
     /_________/  \___/_________/  \___/_________/  \___/_________/  \
     \         \  /   \         \  /   \         \  /   \         \  /
      \_________\/     \_________\/     \_________\/     \_________\/`

func main() {
	fmt.Printf("apex version %v %v \n\n", handlers.GetVersion(), banner)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// after the first signal, restore default handling
		// a second Ctrl+C kills immediately (not gracefull shutdown)
		stop()
	}()

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

	activityStore, err := storage.NewActivityStore(redisURL)
	if err != nil {
		slog.Error("activity store init failed", "error", err)
		os.Exit(1)
	}

	dailyActivityConsumer := consumer.NewDailyActivityConsumer(activityStore)
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		if err := dailyActivityConsumer.Run(ctx); err != nil {
			slog.Error("activity consumer stopped", "error", err)
		}
	}()

	err = startServer(ctx, store)

	switch {
	case err == nil: // ctx cancelled
	case errors.Is(err, server.ErrShutdown):
		slog.Error("graceful shutdown incomplete", "error", err)
	case errors.Is(err, ErrConfig), errors.Is(err, server.ErrOption):
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	default: // ErrServe or unexpected
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}

	// server stopped, closing storage
	if err := store.Close(); err != nil {
		slog.Error("store close failed", "error", err)
	}

	waitFor(consumerDone, "activity consumer", dailyActivityConsumer.MaxStopDelay()+workerStopMargin)
	if err := activityStore.Close(); err != nil {
		slog.Error("activity store close failed", "error", err)
	}
}

func startServer(ctx context.Context, store storage.Storage) error {
	port, err := intFromEnv("PORT", server.DefaultPort)
	if err != nil {
		return err
	}
	shutdownTimeout, err := durationFromEnv("SHUTDOWN_TIMEOUT", server.DefaultShutdownTimeout)
	if err != nil {
		return err
	}
	return server.Start(ctx, server.NewMux(store),
		server.WithPort(port),
		server.WithShutdownTimeout(shutdownTimeout),
	)
}

func waitFor(done <-chan struct{}, name string, timeout time.Duration) {
	select {
	case <-done:
	case <-time.After(timeout):
		slog.Warn("background worker did not stop in time", "worker", name)
	}
}

func intFromEnv(name string, def int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q: %w", ErrConfig, name, v, err)
	}
	return n, nil
}

func durationFromEnv(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q: %w", ErrConfig, name, v, err)
	}
	return d, nil
}
