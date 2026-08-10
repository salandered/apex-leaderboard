package player

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		name string
	}{
		{in: "alice", want: "alice", name: "nothing to trim"},
		{in: "  bob  ", want: "bob", name: "trimmed"},
		{in: "\t\nbob\r\n", want: "bob", name: "trimmed with tabs and newlines"},
		{in: "Mighty Warrior", want: "Mighty Warrior", name: "inner space kept"},
		{in: "   ", want: "", name: "whitespace only"},
		{in: "", want: "", name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeName(tt.in))
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
		name    string
	}{
		{in: "alice", name: "simple name"},
		{in: "Mighty Warrior", name: "space inside"},
		{in: "a b_c-1", name: "all allowed chars"},
		{in: strings.Repeat("a", 32), name: "max len"},

		{in: "", wantErr: true, name: "empty"},
		{in: "ab", wantErr: true, name: "too short"},
		{in: strings.Repeat("a", 33), wantErr: true, name: "too long"},
		{in: "1abc", wantErr: true, name: "digit start"},
		{in: "_abc", wantErr: true, name: "underscore start"},
		{in: "-abc", wantErr: true, name: "hyphen start"},
		{in: "café", wantErr: true, name: "non-ascii"},
		{in: "alice🌳", wantErr: true, name: "emoji"},
		{in: "alice!", wantErr: true, name: "punctuation"},
		{in: "al\nice", wantErr: true, name: "inner newline"},
		{in: "  alice  ", wantErr: true, name: "not normalized"},
		{in: "   ", wantErr: true, name: "whitespace only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewProfileNormalizesThenValidates(t *testing.T) {
	profile, err := NewProfile("  alice  ")
	require.NoError(t, err)
	require.Equal(t, "alice", profile.PlayerName)
	require.NoError(t, profile.PlayerId.Validate())
	require.False(t, profile.CreatedAt.IsZero())

	_, err = NewProfile("  ab  ")
	require.Error(t, err)
}
