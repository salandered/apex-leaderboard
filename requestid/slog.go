package requestid

import (
	"context"
	"log/slog"
)

func LogAttrs(ctx context.Context) []slog.Attr {
	if id := FromContext(ctx); id != "" {
		return []slog.Attr{slog.String("request_id", id)}
	}
	return nil
}
