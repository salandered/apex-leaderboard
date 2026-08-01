package score

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		value   int64
		wantErr bool
	}{
		{name: "zero", value: 0},
		{name: "small positive", value: 67},
		{name: "small negative", value: -67},
		{name: "unix millisecond timestamp", value: 1_754_006_400_000},
		{name: "at max", value: Max},
		{name: "at min", value: Min},
		{name: "one above max", value: Max + 1, wantErr: true},
		{name: "one below min", value: Min - 1, wantErr: true},
		{name: "max int64", value: math.MaxInt64, wantErr: true},
		{name: "min int64", value: math.MinInt64, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.value)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMaxLeavesIntermediateSumsExactInADouble(t *testing.T) {
	const exactIntegerLimit = int64(1) << 53

	require.Less(t, 2*Max, exactIntegerLimit)
}
