// Responsible for a server-generated id of an HTTP request. This request correlation id is used
// in logs, X-Request-ID header and recorded in the score events.
package requestid

import (
	"context"
	"uuid"
)

// TODO: from wavelen: hold the header name here, 'const Header = "X-Request-ID"', and use it in
// requestIDMiddleware and the API tests.

type contextKey int

// "users of WithValue should define their own types for keys ... [like a] an unexported integer type"
const idContextKey contextKey = iota

func New() string {
	return uuid.New().String()
}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idContextKey, id)
}

// returns the id set by the server middleware, or "" if it wasn't run
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(idContextKey).(string)
	return id
}
