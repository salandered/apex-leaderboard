package storage

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExactInt64(t *testing.T) {
	tests := []struct {
		name   string
		raw    float64
		want   int64
		wantOk bool
	}{
		{name: "zero", raw: 0, want: 0, wantOk: true},
		{name: "positive integral", raw: 100, want: 100, wantOk: true},
		{name: "negative integral", raw: -100, want: -100, wantOk: true},
		{name: "largest exact integer in a double", raw: 1 << 53, want: 1 << 53, wantOk: true},
		{name: "min int64 is exactly representable", raw: math.MinInt64, want: math.MinInt64, wantOk: true},
		{name: "fractional", raw: 100.5},
		{name: "tiny fraction", raw: 0.5},
		{name: "not a number", raw: math.NaN()},
		{name: "positive infinity", raw: math.Inf(1)},
		{name: "negative infinity", raw: math.Inf(-1)},
		// float64(math.MaxInt64) rounds up to 2^63, which no int64 holds
		{name: "two to the sixty three", raw: float64(1 << 63)},
		{name: "beyond int64", raw: 1e20},
		{name: "below int64", raw: -1e20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := exactInt64(tt.raw)

			require.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				require.Equal(t, tt.want, v)
			}
		})
	}
}

func TestStreamIDScorePacksSequenceAndRejectsMalformedIDs(t *testing.T) {
	tests := []struct {
		entryID string
		want    float64
		wantErr bool
		name    string
	}{
		{entryID: "1000-0", want: 1000 * seqScale, name: "first entry of a millisecond"},
		{entryID: "1000-2", want: 1000*seqScale + 2, name: "sequence fills the low bits"},
		// packed, so the ZSET cannot tie-break on the member string, where "10" sorts before "2"
		{entryID: "1000-10", want: 1000*seqScale + 10, name: "two digit sequence outranks one digit"},
		{entryID: "1001-0", want: 1001 * seqScale, name: "next millisecond outranks any sequence"},
		{entryID: "1000-5000", want: 1000*seqScale + seqScale - 1, name: "sequence above scale clamps"},
		{entryID: "1000", wantErr: true, name: "no sequence part"},
		{entryID: "abc-0", wantErr: true, name: "bad millisecond part"},
		{entryID: "1000-xyz", wantErr: true, name: "bad sequence part"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, err := streamIDScore(t.Context(), tt.entryID)

			if tt.wantErr {
				require.ErrorIs(t, err, ErrInconsistent)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, score)
		})
	}
}

func TestZScoreToInt64RoundsNonIntegralAndRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		raw    float64
		want   int64
		wantOk bool
	}{
		{name: "integral passes through", raw: 42, want: 42, wantOk: true},
		{name: "rounds up", raw: 41.6, want: 42, wantOk: true},
		{name: "rounds down", raw: 41.4, want: 41, wantOk: true},
		{name: "rounds half away from zero", raw: 41.5, want: 42, wantOk: true},
		{name: "rounds negative half away from zero", raw: -41.5, want: -42, wantOk: true},
		{name: "not a number", raw: math.NaN()},
		{name: "infinity", raw: math.Inf(1)},
		{name: "beyond int64", raw: 1e20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := zScoreToInt64(t.Context(), tt.raw, "app:view:leaderboard:test", "member")

			require.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				require.Equal(t, tt.want, v)
			}
		})
	}
}
