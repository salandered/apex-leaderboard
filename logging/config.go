package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Format selects the log encoding: maps to the [slog.Handler] type.
// Configured via LOG_FORMAT.
// The zero value resolves to [FormatText].
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// TimeFormat selects the text handler's timestamp layout.
// Configured via LOG_TIME.
// The zero value resolves to [TimeDateMilli].
type TimeFormat string

const (
	TimeSec         TimeFormat = "sec"
	TimeMilli       TimeFormat = "milli"
	TimeNano        TimeFormat = "nano"
	TimeDateSec     TimeFormat = "dt-sec"
	TimeDateMilli   TimeFormat = "dt-milli"
	TimeRFC3339     TimeFormat = "rfc3339"
	TimeRFC3339Nano TimeFormat = "rfc3339nano"
)

var timeFormats = []struct {
	name   TimeFormat
	layout string
}{
	{TimeSec, time.TimeOnly},         // 15:04:05
	{TimeMilli, "15:04:05.000"},      // fixed 3 digits
	{TimeNano, "15:04:05.999999999"}, // trailing zeros dropped
	{TimeDateSec, time.DateTime},     // 2006-01-02 15:04:05
	{TimeDateMilli, "2006-01-02 15:04:05.000"},
	{TimeRFC3339, time.RFC3339},         // 2006-01-02T15:04:05Z07:00
	{TimeRFC3339Nano, time.RFC3339Nano}, // same as [FormatJSON] uses
}

// Returns the layout f selects, or "" if f is not a known format.
func (f TimeFormat) layout() string {
	for _, tf := range timeFormats {
		if tf.name == f {
			return tf.layout
		}
	}
	return ""
}

// Config is a logging setup. Its zero value resolves to the documented defaults.
type Config struct {
	Level      slog.Level
	Format     Format
	TimeFormat TimeFormat
	File       string // empty writes to stdout
}

// Fills the empty fields with their defaults and rejects unknown values.
// [Setup] must call it.
// Idempotent.
// Level needs no resolving: its zero value is already the default.
func (c *Config) resolve() error {
	c.Format = Format(normalizeName(string(c.Format)))
	switch c.Format {
	case "":
		c.Format = FormatText
	case FormatText, FormatJSON:
	default:
		return fmt.Errorf("logging: unknown log format %q (LOG_FORMAT; want 'text' or 'json')", c.Format)
	}

	c.TimeFormat = TimeFormat(normalizeName(string(c.TimeFormat)))
	switch {
	case c.TimeFormat == "":
		c.TimeFormat = TimeDateMilli
	case c.TimeFormat.layout() == "":
		return fmt.Errorf("logging: unknown time format %q (LOG_TIME; want one of %s)", c.TimeFormat, timeFormatNames())
	}

	return nil
}

// Env vars:
//   - LOG_LEVEL:  debug | info (default) | warn | error
//   - LOG_FORMAT: text (default) | json
//   - LOG_FILE:   path; empty writes to stdout (default)
//   - LOG_TIME:   sec | milli | nano | dt-sec | dt-milli (default) | rfc3339 | rfc3339nano;
//     text timestamp layout ('dt-' prefixed ones add the date). json is unaffected,
//     it always writes RFC3339Nano.
func ConfigFromEnv() (Config, error) {
	level, err := parseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Level:      level,
		Format:     Format(os.Getenv("LOG_FORMAT")),
		TimeFormat: TimeFormat(os.Getenv("LOG_TIME")),
		File:       os.Getenv("LOG_FILE"),
	}
	if err := cfg.resolve(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch normalizeName(s) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unknown LOG_LEVEL %q (want 'debug', 'info', 'warn', or 'error')", s)
	}
}

// "'sec', 'milli', ..." for error messages; keeps in sync with timeFormats
func timeFormatNames() string {
	quoted := make([]string, len(timeFormats))
	for i, f := range timeFormats {
		quoted[i] = "'" + string(f.name) + "'"
	}
	return strings.Join(quoted, ", ")
}

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
