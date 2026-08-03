package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFormatArgsKeepsShortCommandIntact(t *testing.T) {
	args := []any{"zcard", "app:view:leaderboard:eu"}

	require.Equal(t, "zcard app:view:leaderboard:eu", formatArgs(args))
}

func TestFormatArgsTruncatesASingleLongArg(t *testing.T) {
	// the NOSCRIPT fallback case: the whole Lua source arrives as one arg
	args := []any{"eval", strings.Repeat("x", 5000), "5"}

	require.Equal(t, "eval "+strings.Repeat("x", maxLoggedArg)+"... 5", formatArgs(args))
}

func TestFormatArgsStopsOnceTheTotalBudgetIsSpent(t *testing.T) {
	args := make([]any, 0, 100)
	args = append(args, "mget")
	for range 99 {
		args = append(args, "app:view:leaderboard:eu")
	}

	out := formatArgs(args)

	require.True(t, strings.HasSuffix(out, " ..."), out)
	// one arg may overshoot the total cap, but not a hundred
	require.Less(t, len(out), maxLoggedArgs+maxLoggedArg+len(" ..."))
}

func TestLogCommandReportsRedisNilAsMissNotError(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)
	cmd := redis.NewFloatCmd(t.Context(), "zscore", "app:view:leaderboard:eu", "player-1")

	// when
	err := processWithHook(t, cmd, redis.Nil)

	// then
	require.ErrorIs(t, err, redis.Nil)
	entry := decodeSingleLogLine(t, buf.String())
	require.Equal(t, true, entry["miss"])
	require.NotContains(t, entry, "error")
	require.Equal(t, "zscore", entry["cmd"])
	require.Equal(t, "zscore app:view:leaderboard:eu player-1", entry["args"])
}

func TestLogCommandSkipsTheIdleBlockingLedgerRead(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)
	cmd := redis.NewXStreamSliceCmd(t.Context(), "xread", "block", 5000, "streams", ledgerKey, "0-0")

	// when
	processWithHook(t, cmd, redis.Nil) // the block timed out, no events

	// then
	require.Empty(t, buf.String())
}

func TestLogCommandReportsARealError(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)
	cmd := redis.NewStatusCmd(t.Context(), "ping")

	processWithHook(t, cmd, errors.New("connection refused"))

	entry := decodeSingleLogLine(t, buf.String())
	require.Equal(t, "connection refused", entry["error"])
	require.NotContains(t, entry, "miss")
}

// Runs the command through the hook, with a chain that fails with chainErr.
// Not touching cmd.SetErr: go-redis attaches the error only after the hook
// chain returned (Client.Process), so during the hook cmd.Err() is still nil.
func processWithHook(t *testing.T, cmd redis.Cmder, chainErr error) error {
	t.Helper()

	process := logHook{}.ProcessHook(func(ctx context.Context, cmd redis.Cmder) error {
		return chainErr
	})
	return process(t.Context(), cmd)
}

// Points the default logger at a buffer for the duration of the test.
func captureLogs(t *testing.T, level slog.Leveler) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func decodeSingleLogLine(t *testing.T, raw string) map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	require.Len(t, lines, 1)

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	return entry
}
