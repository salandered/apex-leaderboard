package logging

import (
	"context"
	"log/slog"

	"github.com/salandered/apex/requestid"
)

// key of the correlation id attr added by [contextHandler]
const requestIDKey = "request_id"

// See https://pkg.go.dev/log/slog#example-Handler-LevelHandler
// and https://github.com/golang/example/blob/master/slog-handler-guide/README.md
//
// Wraps a handler to add ctx-scoped attrs to every record.
// slog handlers ignore the ctx by default, so without this every *Context call site
// would have to repeat the correlation id.
type contextHandler struct {
	handler slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	id := requestid.FromContext(ctx)
	if id == "" { // no middleware: a background or direct call
		return h.handler.Handle(ctx, record)
	}
	// The id will be the first attr after the message, so a log is more readable.
	// A new record is needed because Record.AddAttrs can only append.
	out := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	out.AddAttrs(slog.String(requestIDKey, id))
	record.Attrs(func(attr slog.Attr) bool {
		out.AddAttrs(attr)
		return true
	})
	return h.handler.Handle(ctx, out)
}

// Rewraps

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.handler.WithGroup(name)}
}
