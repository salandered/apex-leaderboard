package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// HandleRoot embeds the build-time version, so patch it to a known value here, so
// the assertion is deterministic.
func TestHandleRoot(t *testing.T) {
	original := version
	version = "v-test"

	t.Cleanup(func() { version = original })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	(&HTTPHandler{}).HandleRoot(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "apex v-test — see /api/v1/scores\n", rec.Body.String())
}
