package logging

import (
	"fmt"
	"io"
	"log/slog" // https://go.dev/blog/slog
	"os"

	"github.com/lmittmann/tint"
)

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func Setup(cfg Config) (io.Closer, error) {
	if err := cfg.resolve(); err != nil {
		return nil, err
	}

	var w io.Writer = os.Stdout
	var closer io.Closer = noopCloser{}
	toFile := false
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: open log file: %w", err)
		}
		w = f
		closer = f
		toFile = true
	}

	var h slog.Handler
	if cfg.Format == FormatJSON {
		// uses RFC3339Nano for time
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.Level})
	} else {
		// tint auto drops keys like 'time' and 'level'
		h = tint.NewTextHandler(w, &tint.Options{
			Level:       cfg.Level,
			TimeFormat:  cfg.TimeFormat.layout(), // non-empty after resolve
			NoColor:     toFile,
			ReplaceAttr: customAttr,
		})
	}

	slog.SetDefault(slog.New(contextHandler{h}))

	output := "stdout"
	if toFile {
		output = cfg.File
	}
	slog.Info("logging configured",
		"cfg_level", cfg.Level,
		"cfg_format", string(cfg.Format),
		"cfg_time_format", string(cfg.TimeFormat),
		"cfg_output", output,
	)

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
