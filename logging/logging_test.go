package logging

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"", slog.LevelInfo}, // unset defaults to info
		{"info", slog.LevelInfo},
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn}, // alias
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},   // case insensitive
		{"  warn  ", slog.LevelWarn}, // spaces are trimmed
		{"\tError\n", slog.LevelError},
	}
	for _, c := range cases {
		actual, err := parseLevel(c.in)
		require.NoErrorf(t, err, "input %q", c.in)
		require.Equalf(t, c.want, actual, "input %q", c.in)
	}
}

func TestParseLevelUnknownReturnsError(t *testing.T) {
	for _, in := range []string{"trace", "fatal", "inf", "0"} {
		_, err := parseLevel(in)
		require.Errorf(t, err, "input %q", in)
		require.Containsf(t, err.Error(), in, "error should quote the bad input %q", in)
	}
}

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{
		{"", FormatText}, // unset defaults to text
		{"text", FormatText},
		{"json", FormatJSON},
		{"JSON", FormatJSON},   // case insensitive
		{" text ", FormatText}, // spaces are trimmed
	}
	for _, c := range cases {
		got, err := parseFormat(c.in)
		require.NoErrorf(t, err, "input %q", c.in)
		require.Equalf(t, c.want, got, "input %q", c.in)
	}
}

func TestParseFormatUnknownReturnsError(t *testing.T) {
	for _, in := range []string{"jsonn", "yaml", "txt"} {
		_, err := parseFormat(in)
		require.Errorf(t, err, "input %q", in)
		require.Containsf(t, err.Error(), in, "error should quote the bad input %q", in)
	}
}

func TestParseTimePrecision(t *testing.T) {
	cases := []struct {
		in   string
		want TimePrecision
	}{
		{"", TimeShort}, // unset defaults to short
		{"short", TimeShort},
		{"nano", TimeNano},
		{"NANO", TimeNano},     // case insensitive
		{" short ", TimeShort}, // spaces are trimmed
	}
	for _, c := range cases {
		got, err := parseTimePrecision(c.in)
		require.NoErrorf(t, err, "input %q", c.in)
		require.Equalf(t, c.want, got, "input %q", c.in)
	}
}

func TestParseTimePrecisionUnknownReturnsError(t *testing.T) {
	for _, in := range []string{"long", "rfc3339", "micro"} {
		_, err := parseTimePrecision(in)
		require.Errorf(t, err, "input %q", in)
	}
}

func TestTimePrecisionLayout(t *testing.T) {
	require.Equal(t, time.TimeOnly, TimeShort.layout())
	require.Equal(t, "15:04:05.999999999", TimeNano.layout())
}

func TestZeroConfigUsesDocumentedDefaults(t *testing.T) {
	var cfg Config
	require.Equal(t, slog.LevelInfo, cfg.Level)
	require.Equal(t, FormatText, cfg.Format)
	require.Equal(t, TimeShort, cfg.TimePrecision)
	require.Empty(t, cfg.File) // stdout
	require.Equal(t, time.TimeOnly, cfg.TimePrecision.layout())
}

func TestSetupWritesJsonToLogFile(t *testing.T) {
	restoreDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "apex.log")

	// when
	closer, err := Setup(Config{Level: slog.LevelWarn, Format: FormatJSON, File: path})
	require.NoError(t, err)

	slog.Info("filtered out by the warn level")
	slog.Warn("written", "board_id", "main")
	require.NoError(t, closer.Close())

	// then
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := splitNonEmptyLines(string(content))
	require.Len(t, lines, 1) // the info line is below the configured level

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	require.Equal(t, "WARN", entry["level"])
	require.Equal(t, "written", entry["msg"])
	require.Equal(t, "main", entry["board_id"])
}

func TestSetupAppendsToExistingLogFileNotTruncate(t *testing.T) {
	restoreDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "apex.log")
	require.NoError(t, os.WriteFile(path, []byte("previous run\n"), 0o644))

	// when
	closer, err := Setup(Config{Format: FormatJSON, File: path})
	require.NoError(t, err)
	slog.Info("second run")
	require.NoError(t, closer.Close())

	// then
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := splitNonEmptyLines(string(content))
	require.Len(t, lines, 2)
	require.Equal(t, "previous run", lines[0])
}

func TestConfigFromEnvReadsAllVars(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_TIME", "nano")
	t.Setenv("LOG_FILE", "/tmp/apex.log")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, Config{
		Level:         slog.LevelDebug,
		Format:        FormatJSON,
		TimePrecision: TimeNano,
		File:          "/tmp/apex.log",
	}, cfg)
}

func TestConfigFromEnvUnknownLevelReturnsError(t *testing.T) {
	t.Setenv("LOG_LEVEL", "trace")

	_, err := ConfigFromEnv()
	require.Error(t, err)
}

func TestConfigFromEnvUnknownFormatReturnsError(t *testing.T) {
	t.Setenv("LOG_FORMAT", "jsonn")

	_, err := ConfigFromEnv()
	require.Error(t, err)
}

func TestConfigFromEnvUnknownTimeReturnsError(t *testing.T) {
	t.Setenv("LOG_TIME", "long")

	_, err := ConfigFromEnv()
	require.Error(t, err)
}

// puts global default logger back when the test ends
func restoreDefaultLogger(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
