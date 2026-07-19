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
