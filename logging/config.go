package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Format selects the log encoding. Maps to the [slog.Handler] type.
// The zero value is text.
type Format int

const (
	FormatText Format = iota
	FormatJSON
)

// mirrors the LOG_FORMAT vocabulary
func (f Format) String() string {
	if f == FormatJSON {
		return "json"
	}
	return "text"
}

// TimePrecision selects the text handler's timestamp layout. The zero value is short.
type TimePrecision int

const (
	TimeShort TimePrecision = iota
	TimeNano
)

// mirrors the LOG_TIME vocabulary
func (p TimePrecision) String() string {
	if p == TimeNano {
		return "nano"
	}
	return "short"
}

// Picks the text handler's timestamp layout;
// TimeNano (1) mirrors json's RFC3339Nano precision (trailing zeros dropped) but keeps only the time part
func (p TimePrecision) layout() string {
	if p == TimeNano {
		return "15:04:05.999999999"
	}
	return time.TimeOnly
}

// Config is a logging setup. Its zero value is the documented default.
type Config struct {
	Level         slog.Level
	Format        Format
	TimePrecision TimePrecision
	File          string // empty writes to stdout
}

// Env vars:
//   - LOG_LEVEL:  debug | info (default) | warn | error
//   - LOG_FORMAT: text (default) | json
//   - LOG_FILE:   path; empty writes to stdout (default)
//   - LOG_TIME:   short (default) | nano; text timestamp precision (json unaffected)
func ConfigFromEnv() (Config, error) {
	level, err := parseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return Config{}, err
	}

	format, err := parseFormat(os.Getenv("LOG_FORMAT"))
	if err != nil {
		return Config{}, err
	}

	precision, err := parseTimePrecision(os.Getenv("LOG_TIME"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Level:         level,
		Format:        format,
		TimePrecision: precision,
		File:          os.Getenv("LOG_FILE"),
	}, nil
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
		return 0, fmt.Errorf("logging: unknown LOG_LEVEL %q (want 'debug', 'info', 'warn', or 'error')", s)
	}
}

func parseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	default:
		return 0, fmt.Errorf("logging: unknown LOG_FORMAT %q (want 'text' or 'json')", s)
	}
}

func parseTimePrecision(s string) (TimePrecision, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "short":
		return TimeShort, nil
	case "nano":
		return TimeNano, nil
	default:
		return 0, fmt.Errorf("logging: unknown LOG_TIME %q (want 'short' or 'nano')", s)
	}
}
