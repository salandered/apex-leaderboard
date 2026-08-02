package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/salandered/apex/requestid"
	"github.com/stretchr/testify/require"
)

func TestSetupWritesJsonToLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apex.log")
	setupForTest(t, Config{Level: slog.LevelWarn, Format: FormatJSON, File: path})

	// when
	slog.Info("filtered out by the warn level")
	slog.Warn("written", "board_id", "main")

	// then
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	entry := decodeLogLine(t, string(content)) // single line: the info one is below the level
	require.Equal(t, "WARN", entry["level"])
	require.Equal(t, "written", entry["msg"])
	require.Equal(t, "main", entry["board_id"])
}

func TestSetupAppendsToExistingLogFileNotTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apex.log")
	require.NoError(t, os.WriteFile(path, []byte("previous run\n"), 0o644))
	setupForTest(t, Config{Format: FormatJSON, File: path})

	// when
	slog.Info("second run")

	// then
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := splitNonEmptyLines(string(content))
	require.Len(t, lines, 2)
	require.Equal(t, "previous run", lines[0])
}

func TestContextHandlerAddsRequestIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(contextHandler{slog.NewJSONHandler(&buf, nil)})

	ctx := requestid.WithID(context.Background(), "req-42")

	// when
	logger.InfoContext(ctx, "written", "board_id", "main")

	// then
	entry := decodeLogLine(t, buf.String())
	require.Equal(t, "req-42", entry[requestIDKey])
	require.Equal(t, "main", entry["board_id"])
}

func TestContextHandlerOmitsRequestIDWhenContextHasNone(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(contextHandler{slog.NewJSONHandler(&buf, nil)})

	logger.InfoContext(context.Background(), "written")

	require.NotContains(t, decodeLogLine(t, buf.String()), requestIDKey)
}

// slog.With uses WithAttrs which returns an inner handler if not wrapped correctly
func TestContextHandlerKeepsRequestIDAfterWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(contextHandler{slog.NewJSONHandler(&buf, nil)}).With("component", "storage")

	ctx := requestid.WithID(context.Background(), "req-42")

	// when
	logger.InfoContext(ctx, "written")

	// then
	entry := decodeLogLine(t, buf.String())
	require.Equal(t, "req-42", entry[requestIDKey])
	require.Equal(t, "storage", entry["component"])
}

func TestSetupWrapsHandlerForRequestIDCorrelation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apex.log")
	setupForTest(t, Config{Format: FormatJSON, File: path})

	ctx := requestid.WithID(context.Background(), "req-42")
	slog.InfoContext(ctx, "written")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "req-42", decodeLogLine(t, string(content))[requestIDKey])
}

// 'Setup' must not be called directly.
// Saves default logger, setups the a new one from config and then restores default.
func setupForTest(t *testing.T, cfg Config) {
	t.Helper()

	previous := slog.Default()
	closer, err := Setup(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		slog.SetDefault(previous)
		// The log file is closed here and not in the test body
		if err := closer.Close(); err != nil {
			t.Errorf("error while closing logger %v", err)
		}
	})
}

// unmarshals a single json log line, failing if the input holds anything else
func decodeLogLine(t *testing.T, raw string) map[string]any {
	// example (happy):
	// 		raw is `{"level":"info","msg":"User logged in","user_id":123}`
	// 		splitNonEmptyLines returns ["{\"level\":\"info\",...}"]
	// 		json.Unmarshal -> map[string]any{"level":"info", "msg":"User logged in", "user_id":123}

	t.Helper()

	lines := splitNonEmptyLines(raw)
	require.Len(t, lines, 1)

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	return entry
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
