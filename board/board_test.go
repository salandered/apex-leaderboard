package board

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIDValidate(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
		name    string
	}{
		{id: "main", name: "simple id"},
		{id: "2026", name: "all digits"},
		{id: "summer-contest-2026", name: "with hyphens"},
		{id: "1main", name: "digit start"},
		{id: "main1", name: "digit end"},

		{id: "ab", wantErr: true, name: "too short"},
		{id: strings.Repeat("a", 33), wantErr: true, name: "too long"},
		{id: "", wantErr: true, name: "empty"},
		{id: "Main", wantErr: true, name: "uppercase"},
		{id: "summer_contest", wantErr: true, name: "underscore"},
		{id: "boards:main", wantErr: true, name: "colon"},
		{id: "sum mer", wantErr: true, name: "space"},
		{id: "-abc", wantErr: true, name: "leading hyphen"},
		{id: "abc-", wantErr: true, name: "trailing hyphen"},
		{id: "a--b", wantErr: true, name: "consecutive hyphens"},
		{id: "a-----b", wantErr: true, name: "consecutive hyphens many"},
		{id: "café-board", wantErr: true, name: "non-ascii"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ID(tt.id).Validate()

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		name string
	}{
		{in: "Summer Contest", want: "Summer Contest", name: "all ok"},
		{in: "  Main Cup  ", want: "Main Cup", name: "trimmed"},
		{in: "\t\nMain Cup\r\n", want: "Main Cup", name: "trimmed with tabs and newlines"},
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
		{in: "Summer Contest", name: "simple name"},
		{in: "Demo Cup 🌳", name: "with emoji"},
		{in: "サマー", name: "cjk"},
		{in: "café", name: "accent"},
		{in: "👨‍👩‍👧‍👦", name: "single zwj emoji (7 runes)"},
		{in: strings.Repeat("a", 32), name: "max len"},
		{in: strings.Repeat("サ", 32), name: "max len counts runes not bytes"},

		{in: "", wantErr: true, name: "empty"},
		{in: "F1", wantErr: true, name: "too short"},
		{in: "🏆", wantErr: true, name: "single emoji is too short"},
		{in: strings.Repeat("a", 33), wantErr: true, name: "too long"},
		{in: strings.Repeat("サ", 33), wantErr: true, name: "too long in runes"},
		{in: "a\nb cup", wantErr: true, name: "inner newline"},
		{in: "a\tb cup", wantErr: true, name: "inner tab"},
		{in: "a\x00b cup", wantErr: true, name: "inner nul"},
		{in: "  Main Cup  ", wantErr: true, name: "not normalized"},
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

func TestNewBoardNormalizesThenValidates(t *testing.T) {
	b, err := NewBoard("summer-cup", "  Main Cup  ", "")
	require.NoError(t, err)
	require.Equal(t, ID("summer-cup"), b.BoardId)
	require.Equal(t, "Main Cup", b.BoardName)
	require.Equal(t, BoardActive, b.State, "empty state defaults to active")
	require.False(t, b.CreatedAt.IsZero())

	_, err = NewBoard("summer-cup", "F1", "")
	require.Error(t, err, "invalid name")

	_, err = NewBoard("Summer Cup", "Main Cup", "")
	require.Error(t, err, "invalid board id")

	_, err = NewBoard("summer-cup", "Main Cup", "paused")
	require.Error(t, err, "invalid state")
}
