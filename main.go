package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/salandered/apex/apexredis"
	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/server"
	"github.com/salandered/apex/storage"
)

var ErrConfig = errors.New("invalid config")

const workerStopMargin = 2 * time.Second

const banner = `
       _________        _________        _________        _________
      /    A    /\     /    P    /\     /    E    /\     /    X    /\
     /_________/  \___/_________/  \___/_________/  \___/_________/  \
     \         \  /   \         \  /   \         \  /   \         \  /
      \_________\/     \_________\/     \_________\/     \_________\/`

func main() {
	cfg, logCloser, err := setupLogging()
	if err != nil {
		// logger isn't ready yet, report to stderr directly
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = logCloser.Close() }()

	// decoration, not a log record
	if cfg.Format == logging.FormatText && cfg.File == "" {
		fmt.Printf("%v\n\n", banner)
	}

	slog.Info("apex starting", "version", handlers.GetVersion())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// after the first signal, restore default handling
		// a second Ctrl+C kills immediately (not a gracefull shutdown)
		stop()
	}()

	redisCfg, err := apexredis.ConfigFromEnv()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Redis clients are built here and are closed here in main.

	requestClient, err := apexredis.New(redisCfg, "storage")
	if err != nil {
		slog.Error("redis client init failed", "error", err)
		os.Exit(1)
	}

	// A second client: the consumer's blocking XREAD holds its connection,
	// should not interfere with the requestClient.
	consumerClient, err := apexredis.New(redisCfg, "consumer")
	if err != nil {
		slog.Error("ledger redis client init failed", "error", err)
		os.Exit(1)
	}

	store := storage.NewStorage(requestClient)

	// Every view derived from the ledger is its own consumer.
	// Each one blocks on XREAD, but only while idle.
	// Sharing one client is ok.
	consumers := []*consumer.Consumer{
		consumer.NewDailyActivityConsumer(storage.NewActivityStore(consumerClient)),
		consumer.NewPlayerHistoryConsumer(storage.NewPlayerHistoryStore(consumerClient)),
	}
	consumersDone := make(chan struct{})
	go func() {
		defer close(consumersDone)
		var wg sync.WaitGroup
		for _, c := range consumers {
			wg.Go(func() {
				if err := c.Run(ctx); err != nil {
					slog.Error("consumer failed", "error", err)
				}
			})
		}
		wg.Wait()
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

	// server stopped, closing the request pool
	if err := requestClient.Close(); err != nil {
		slog.Error("redis client close failed", "error", err)
	}

	// Close the ledger pool after all consumers stopped.
	// Using the slowest one for decision.
	waitFor(
		consumersDone,
		"ledger consumers",
		maxStopDelay(consumers)+workerStopMargin,
	)
	if err := consumerClient.Close(); err != nil {
		slog.Error("ledger redis client close failed", "error", err)
	}
}

// Returns the resolved config too: it tells whether stdout is human readable.
func setupLogging() (logging.Config, io.Closer, error) {
	cfg, err := logging.ConfigFromEnv()
	if err != nil {
		return logging.Config{}, nil, err
	}
	closer, err := logging.Setup(cfg)
	if err != nil {
		return logging.Config{}, nil, err
	}
	return cfg, closer, nil
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

func maxStopDelay(consumers []*consumer.Consumer) time.Duration {
	var slowest time.Duration
	for _, c := range consumers {
		slowest = max(slowest, c.BlockDuration)
	}
	return slowest
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
