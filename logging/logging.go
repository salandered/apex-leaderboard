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

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

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
	var closer io.Closer = noopCloser{}
	toFile := false
	if path := os.Getenv("LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: open log file: %w", err)
		}
		w = f
		closer = f
		toFile = true
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
			ReplaceAttr: customAttr,
		})
	}

	slog.SetDefault(slog.New(h))
	return closer, nil
}

// maps levels to tint's built-in coloring;
// no Debug
var tintLevelColors = map[slog.Level]uint8{
	slog.LevelInfo:  10, // bright green
	slog.LevelWarn:  11, // bright yellow
	slog.LevelError: 9,  // bright red

}

// the http status attr colored;
// no 1xx
var tintStatusColors = []struct {
	min   int64
	color uint8
}{
	{500, 9},  // bright red
	{400, 11}, // bright yellow
	{300, 13}, // bright magenta
	{200, 10}, // bright green
}

// key of the http status attr colored by [tintStatus]
const statusKey = "status"

/*
from docs https://pkg.go.dev/log/slog#HandlerOptions

The built-in attributes with keys "time", "level", "source", and "msg" are passed to this function.

The first argument is a list of currently open groups that contain the Attr.
For example, the attribute list

	Int("a", 1), Group("g", Int("b", 2)), Int("c", 3)

results in consecutive calls to ReplaceAttr with the following arguments:

	nil, Int("a", 1)
	[]string{"g"}, Int("b", 2)
	nil, Int("c", 3)
*/
func customAttr(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) != 0 {
		return attr
	}
	switch attr.Key {
	case slog.LevelKey:
		return fullLevelTintedName(attr)
	case statusKey:
		return tintStatus(attr)
	default:
		return attr
	}
}

// Renders the level as its full name (INFO, not INF)
// Makes a tint.
func fullLevelTintedName(attr slog.Attr) slog.Attr {
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

// Tints the status code (e.g. 2xx are green)
func tintStatus(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindInt64 {
		return attr
	}
	code := attr.Value.Int64()
	for _, c := range tintStatusColors {
		if code >= c.min {
			return tint.Attr(c.color, attr)
		}
	}
	return attr
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
