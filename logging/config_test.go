package logging

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigFromEnvReadsAllVars(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_TIME", "nano")
	t.Setenv("LOG_FILE", "/tmp/apex.log")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, Config{
		Level:      slog.LevelDebug,
		Format:     FormatJSON,
		TimeFormat: TimeNano,
		File:       "/tmp/apex.log",
	}, cfg)
}

func TestZeroConfigResolvesToDocumentedDefaults(t *testing.T) {
	var cfg Config
	require.NoError(t, cfg.resolve())
	require.Equal(t, slog.LevelInfo, cfg.Level)
	require.Equal(t, FormatText, cfg.Format)
	require.Equal(t, TimeDateMilli, cfg.TimeFormat)
	require.Empty(t, cfg.File) // stdout
	require.Equal(t, "2006-01-02 15:04:05.000", cfg.TimeFormat.layout())
}

func TestResolveKeepsKnownValues(t *testing.T) {
	cfg := Config{Level: slog.LevelWarn, Format: FormatJSON, TimeFormat: TimeSec, File: "/tmp/apex.log"}
	before := cfg

	require.NoError(t, cfg.resolve())
	require.Equal(t, before, cfg)
}

func TestResolveIsIdempotent(t *testing.T) {
	var cfg Config
	require.NoError(t, cfg.resolve())
	once := cfg

	require.NoError(t, cfg.resolve())
	require.Equal(t, once, cfg)
}

func TestResolveTrimsAndLowercasesNames(t *testing.T) {
	cfg := Config{Format: " JSON ", TimeFormat: "\tDT-Milli\n"}
	require.NoError(t, cfg.resolve())
	require.Equal(t, FormatJSON, cfg.Format)
	require.Equal(t, TimeDateMilli, cfg.TimeFormat)
}

func TestResolveUnknownValuesReturnError(t *testing.T) {
	for _, cfg := range []Config{
		{Format: "yaml"},
		{TimeFormat: "hh:mm:ss"},
		{TimeFormat: "dt-nano"},
	} {
		require.Errorf(t, cfg.resolve(), "config %+v", cfg)
	}
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

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		in   Format
		want Format
	}{
		{"", FormatText}, // unset defaults to text
		{"text", FormatText},
		{"json", FormatJSON},
		{"JSON", FormatJSON},   // case insensitive
		{" text ", FormatText}, // spaces are trimmed
	}
	for _, c := range cases {
		cfg := Config{Format: c.in}
		require.NoErrorf(t, cfg.resolve(), "input %q", c.in)
		require.Equalf(t, c.want, cfg.Format, "input %q", c.in)
	}
}

func TestResolveUnknownFormatReturnsError(t *testing.T) {
	for _, in := range []Format{"jsonn", "yaml", "txt"} {
		cfg := Config{Format: in}
		err := cfg.resolve()
		require.Errorf(t, err, "input %q", in)
		require.Containsf(t, err.Error(), string(in), "error should quote the bad input %q", in)
	}
}

func TestResolveTimeFormat(t *testing.T) {
	cases := []struct {
		in   TimeFormat
		want TimeFormat
	}{
		{"", TimeDateMilli}, // unset defaults to dt-milli
		{"sec", TimeSec},
		{"milli", TimeMilli},
		{"nano", TimeNano},
		{"dt-sec", TimeDateSec},
		{"dt-milli", TimeDateMilli},
		{"rfc3339", TimeRFC3339},
		{"rfc3339nano", TimeRFC3339Nano},
		{"DT-MILLI", TimeDateMilli}, // case insensitive
		{" milli ", TimeMilli},      // spaces are trimmed
	}
	for _, c := range cases {
		cfg := Config{TimeFormat: c.in}
		require.NoErrorf(t, cfg.resolve(), "input %q", c.in)
		require.Equalf(t, c.want, cfg.TimeFormat, "input %q", c.in)
	}
}

func TestResolveUnknownTimeFormatReturnsError(t *testing.T) {
	for _, in := range []TimeFormat{"short", "long", "micro", "dt", "dt-nano", "rfc3339milli"} {
		cfg := Config{TimeFormat: in}
		err := cfg.resolve()
		require.Errorf(t, err, "input %q", in)
		require.Containsf(t, err.Error(), string(in), "error should quote the bad input %q", in)
	}
}

func TestEveryTimeFormatNameResolves(t *testing.T) {
	for _, tf := range timeFormats {
		cfg := Config{TimeFormat: tf.name}
		require.NoErrorf(t, cfg.resolve(), "format %s", tf.name)
		require.Equalf(t, tf.name, cfg.TimeFormat, "format %s", tf.name)
		require.Containsf(t, timeFormatNames(), string(tf.name), "format %s must be listed in errors", tf.name)
	}
}

// guards against a typo in a layout, which would otherwise print literally
func TestTimeFormatLayoutRendersReferenceTime(t *testing.T) {
	ts := time.Date(2026, 8, 3, 5, 23, 40, 123456789, time.UTC)
	cases := []struct {
		format TimeFormat
		want   string
	}{
		{TimeSec, "05:23:40"},
		{TimeMilli, "05:23:40.123"},
		{TimeNano, "05:23:40.123456789"},
		{TimeDateSec, "2026-08-03 05:23:40"},
		{TimeDateMilli, "2026-08-03 05:23:40.123"},
		{TimeRFC3339, "2026-08-03T05:23:40Z"},
		{TimeRFC3339Nano, "2026-08-03T05:23:40.123456789Z"},
	}
	require.Len(t, cases, len(timeFormats), "every format needs a case")
	for _, c := range cases {
		require.Equalf(t, c.want, ts.Format(c.format.layout()), "format %s", c.format)
	}
}

func TestUnknownTimeFormatHasNoLayout(t *testing.T) {
	require.Empty(t, TimeFormat("").layout())
	require.Empty(t, TimeFormat("kitchen").layout())
	require.Empty(t, TimeFormat("dt-nano").layout())
}
