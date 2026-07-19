package handlers

import (
	"fmt"
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

	HandleRoot(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "apex v-test — see /api/v1/scores\n", rec.Body.String())
}

func TestParseIntQuery(t *testing.T) {
	tests := []struct {
		name      string
		raw       string // omitted from the URL when empty
		def       int64
		min       int64
		max       int64 // <= 0 means no cap
		wantValue int64
		wantErr   bool
	}{
		{name: "missing param returns default", raw: "", def: 10, min: 0, max: 100, wantValue: 10},
		{name: "value within range", raw: "5", def: 10, min: 0, max: 100, wantValue: 5},
		{name: "value at min boundary", raw: "0", def: 10, min: 0, max: 100, wantValue: 0},
		{name: "value at max boundary", raw: "100", def: 10, min: 0, max: 100, wantValue: 100},
		{name: "no cap allows large value", raw: "1000000", def: 10, min: 0, max: 0, wantValue: 1000000},
		{name: "not an integer", raw: "abc", def: 10, min: 0, max: 100, wantErr: true},
		{name: "below min", raw: "-1", def: 10, min: 0, max: 100, wantErr: true},
		{name: "above max", raw: "101", def: 10, min: 0, max: 100, wantErr: true},
		{name: "below min with no cap", raw: "-1", def: 10, min: 0, max: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.raw != "" {
				url = fmt.Sprintf("/?%s=%s", "param", tt.raw)
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			v, err := parseIntQuery(req, "param", tt.def, tt.min, tt.max)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantValue, v)
		})
	}
}
