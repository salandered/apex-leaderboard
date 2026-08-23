package requestid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogAttrsReturnsRequestIDFromContext(t *testing.T) {
	ctx := WithID(context.Background(), "req-42")

	attrs := LogAttrs(ctx)

	require.Len(t, attrs, 1)
	require.Equal(t, "request_id", attrs[0].Key)
	require.Equal(t, "req-42", attrs[0].Value.String())
}

func TestLogAttrsReturnsNothingWhenContextHasNoID(t *testing.T) {
	require.Empty(t, LogAttrs(context.Background()))
}

// an id set to "" should not reach a record
func TestLogAttrsReturnsNothingForAnEmptyID(t *testing.T) {
	require.Empty(t, LogAttrs(WithID(context.Background(), "")))
}
