package logging

import (
	"fmt"
	"io"
	"log/slog" // https://go.dev/blog/slog
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

// TODO make env options as consts

// Env vars (all optional):
//   - LOG_LEVEL:  debug | info (default) | warn | error
//   - LOG_FORMAT: text (default) | json
//   - LOG_FILE:   path; empty writes to stdout (default)
//   - LOG_TIME:   short (default) | nano; text timestamp precision (json unaffected)
func Setup() (io.Closer, error) {

	level, err := parseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return nil, err
	}

	timeFormat, err := parseTimeFormat(os.Getenv("LOG_TIME"))
	if err != nil {
		return nil, err
	}

	var w io.Writer = os.Stdout
	closer := io.Closer(io.NopCloser(nil)) // no-op for stdout
	toFile := false
	if path := os.Getenv("LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: open log file: %w", err)
		}
		w, closer, toFile = f, f, true
	}

	var h slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "json" {
		// uses RFC3339Nano for time
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	} else {
		// tint auto drops keys like 'time' and 'level'
		h = tint.NewTextHandler(w, &tint.Options{
			Level:       level,
			TimeFormat:  timeFormat,
			NoColor:     toFile,
			ReplaceAttr: fullLevelName,
		})
	}

	slog.SetDefault(slog.New(h))
	return closer, nil
}

// maps levels to tint's built-in coloring;
// no Debug: tint leaves it uncolored
var tintLevelColors = map[slog.Level]uint8{
	slog.LevelInfo:  10, // bright green
	slog.LevelWarn:  11, // bright yellow
	slog.LevelError: 9,  // bright red

}

// renders the level as its full name (INFO, not INF)
func fullLevelName(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) != 0 || attr.Key != slog.LevelKey {
		return attr
	}
	level, ok := attr.Value.Any().(slog.Level)
	if !ok {
		return attr
	}
	named := slog.String(attr.Key, level.String())
	if color, ok := tintLevelColors[level]; ok {
		return tint.Attr(color, named)
	}
	return named
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unknown LOG_LEVEL %q (want debug, info, warn, or error)", s)
	}
}

// picks the text handler's timestamp layout; 'nano' mirrors json's RFC3339Nano
// precision (trailing zeros dropped) but keeps only the time part
func parseTimeFormat(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "short":
		return time.TimeOnly, nil
	case "nano":
		return "15:04:05.999999999", nil
	default:
		return "", fmt.Errorf("logging: unknown LOG_TIME %q (want short or nano)", s)
	}
}
