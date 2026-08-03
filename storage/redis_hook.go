package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const logComponent = "storage"

// Automatically logs every command and pipeline at Debug.
type logHook struct{}

var _ redis.Hook = logHook{} // should satisfy [redis.Hook]

// go-redis already logs dial errors to its own logger (see [redisLogger])
func (logHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (logHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		// just wrap when not debugging
		if !slog.Default().Enabled(ctx, slog.LevelDebug) {
			return next(ctx, cmd)
		}

		start := time.Now()
		err := next(ctx, cmd)
		logCommand(ctx, cmd, time.Since(start))
		return err
	}
}

func (logHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		// just wrap when not debugging
		if !slog.Default().Enabled(ctx, slog.LevelDebug) {
			return next(ctx, cmds)
		}

		start := time.Now()
		err := next(ctx, cmds)

		// one line per pipeline, not per command
		attrs := []slog.Attr{
			slog.String("component", logComponent),
			slog.Int("n", len(cmds)),
			slog.String("cmds", cmdNames(cmds)),
			slog.Duration("dur", time.Since(start)),
		}
		if err != nil && !errors.Is(err, redis.Nil) { // a miss inside the pipeline is not a failure
			attrs = append(attrs, slog.String("error", err.Error()))
		}
		slog.LogAttrs(ctx, slog.LevelDebug, "redis pipeline", attrs...)
		return err
	}
}

// The level stays Debug even on error: the caller wraps and reports the failure itself,
// a second Error line for the same fault is noise.
func logCommand(ctx context.Context, cmd redis.Cmder, dur time.Duration) {
	err := cmd.Err()
	// redis.Nil is a plain miss, not an error (see docs)
	miss := errors.Is(err, redis.Nil)

	// XREAD BLOCK blocks for [consumer.blockDuration] seconds.
	// If not new events, Redis holds the connection and then answers empty (redis.Nil)
	// So without this branch we'll get repeating logs like this:
	// DEBUG redis cmd op=xread args="xread block ..." dur=5.001s
	// DEBUG redis cmd op=xread ... dur=5.002s
	if miss && cmd.Name() == "xread" {
		return
	}

	attrs := []slog.Attr{
		slog.String("component", logComponent),
		slog.String("cmd", cmd.FullName()),
		slog.String("args", formatArgs(cmd.Args())),
		slog.Duration("dur", dur),
	}
	if miss {
		attrs = append(attrs, slog.Bool("miss", true))
	} else if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	slog.LogAttrs(ctx, slog.LevelDebug, "redis cmd", attrs...)
}

const (
	maxLoggedArg  = 64  // per arg
	maxLoggedArgs = 256 // per command (the last arg may exceed it)
)

// Renders the command.
// The per-arg cap: a NOSCRIPT fallback resends the whole Lua source as a single arg (see docs)
func formatArgs(args []any) string {
	var b strings.Builder
	for i, arg := range args {
		if b.Len() >= maxLoggedArgs {
			b.WriteString(" ...")
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(truncateArg(fmt.Sprint(arg)))
	}
	return b.String()
}

func cmdNames(cmds []redis.Cmder) string {
	var b strings.Builder
	for i, cmd := range cmds {
		if b.Len() >= maxLoggedArgs {
			b.WriteString(" ...")
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(cmd.FullName())
	}
	return b.String()
}

func truncateArg(arg string) string {
	if len(arg) <= maxLoggedArg {
		return arg
	}
	// args may carry raw UTF-8 (like player names), so just a cut might split a rune
	return strings.ToValidUTF8(arg[:maxLoggedArg], "") + "..."
}
