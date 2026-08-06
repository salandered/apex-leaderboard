package board

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "simple id", id: "main"},
		{name: "all digits", id: "2026"},
		{name: "with hyphens", id: "summer-contest-2026"},
		{name: "digit start", id: "1main"},
		{name: "digit end", id: "main1"},

		{name: "too short", id: "ab", wantErr: true},
		{name: "too long", id: strings.Repeat("a", 33), wantErr: true},
		{name: "empty", id: "", wantErr: true},
		{name: "uppercase", id: "Main", wantErr: true},
		{name: "underscore", id: "summer_contest", wantErr: true},
		{name: "colon", id: "boards:main", wantErr: true},
		{name: "space", id: "sum mer", wantErr: true},
		{name: "leading hyphen", id: "-abc", wantErr: true},
		{name: "trailing hyphen", id: "abc-", wantErr: true},
		{name: "consecutive hyphens", id: "a--b", wantErr: true},
		{name: "consecutive hyphens many", id: "a-----b", wantErr: true},
		{name: "non-ascii", id: "café-board", wantErr: true},
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
		in      string
		want    string
		wantErr bool
		name    string
	}{
		{in: "Summer Contest", want: "Summer Contest", name: "simple name"},
		{in: "Demo Cup 🌳", want: "Demo Cup 🌳", name: "with emoji"},
		{in: "サマー", want: "サマー", name: "cjk"},
		{in: "café", want: "café", name: "accent"},
		{in: "👨‍👩‍👧‍👦", want: "👨‍👩‍👧‍👦", name: "single zwj emoji (7 runes)"},
		{in: "  Main Cup  ", want: "Main Cup", name: "trimmed"},
		{in: strings.Repeat("a", 32), want: strings.Repeat("a", 32), name: "max len"},
		{in: strings.Repeat("サ", 32), want: strings.Repeat("サ", 32), name: "max len counts runes not bytes"},

		{in: "", wantErr: true, name: "empty"},
		{in: "   ", wantErr: true, name: "whitespace only"},
		{in: "F1", wantErr: true, name: "too short"},
		{in: "🏆", wantErr: true, name: "single emoji is too short"},
		{in: strings.Repeat("a", 33), wantErr: true, name: "too long"},
		{in: strings.Repeat("サ", 33), wantErr: true, name: "too long in runes"},
		{in: "a\nb cup", wantErr: true, name: "inner newline"},
		{in: "a\tb cup", wantErr: true, name: "inner tab"},
		{in: "a\x00b cup", wantErr: true, name: "inner nul"},
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
