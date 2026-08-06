package player

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
		name    string
	}{
		{in: "alice", want: "alice", name: "simple name"},
		{in: "Mighty Warrior", want: "Mighty Warrior", name: "space inside"},
		{in: "a b_c-1", want: "a b_c-1", name: "all allowed chars"},
		{in: "  bob  ", want: "bob", name: "trimmed"},
		{in: "\t\nbob\r\n", want: "bob", name: "trimmed with tabs and newlines"},
		{in: strings.Repeat("a", 32), want: strings.Repeat("a", 32), name: "max len"},
		{in: "  " + strings.Repeat("a", 32) + "  ", want: strings.Repeat("a", 32), name: "max len after trim"},

		{in: "", wantErr: true, name: "empty"},
		{in: "   ", wantErr: true, name: "whitespace only"},
		{in: "ab", wantErr: true, name: "too short"},
		{in: "  ab  ", wantErr: true, name: "too short after trim"},
		{in: strings.Repeat("a", 33), wantErr: true, name: "too long"},
		{in: "1abc", wantErr: true, name: "digit start"},
		{in: "_abc", wantErr: true, name: "underscore start"},
		{in: "-abc", wantErr: true, name: "hyphen start"},
		{in: "café", wantErr: true, name: "non-ascii"},
		{in: "alice🌳", wantErr: true, name: "emoji"},
		{in: "alice!", wantErr: true, name: "punctuation"},
		{in: "al\nice", wantErr: true, name: "inner newline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeName(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
