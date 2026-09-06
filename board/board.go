package board

import (
	"fmt"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/strvalid"
)

var (
	// a-z, 0-9 and inner single hyphens; len is 3-32
	idConfig = strvalid.Config{
		Subject: "board id",
		MinLen:  3,
		MaxLen:  32,

		Digits: true,
		Dash:   strvalid.SepInner,

		EchoValue: true,
	}

	// Unicode is allowed except for control chars; len is 3-32 runes.
	nameConfig = strvalid.UnicodeConfig{
		Subject:  "board name",
		MinRunes: 3,
		MaxRunes: 32,

		EchoValue: true,
	}
)

const (
	BoardActive BoardState = "active"
	BoardClosed BoardState = "closed"
)

type Board struct {
	BoardId   ID
	BoardName string
	State     BoardState
	CreatedAt time.Time
}

type ID string

func (id ID) String() string {
	return string(id)
}

func (id ID) Validate() error {
	return strvalid.Validate(string(id), idConfig)
}

type BoardState string

func (state BoardState) Validate() error {
	switch state {
	case BoardActive, BoardClosed:
		return nil
	default:
		return fmt.Errorf("invalid board state %q", state)
	}
}

// Normalizes and validates provided fields; fills the generated ones.
// An empty state means active.
func NewBoard(boardId ID, boardName string, state BoardState) (*Board, error) {
	if err := boardId.Validate(); err != nil {
		return nil, err
	}
	boardName = NormalizeName(boardName)
	if err := ValidateName(boardName); err != nil {
		return nil, err
	}
	if state == "" {
		state = BoardActive
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	return &Board{
		BoardId:   boardId,
		BoardName: boardName,
		State:     state,
		CreatedAt: apextime.Now(),
	}, nil
}

// Trims surrounding spaces.
func NormalizeName(name string) string {
	return strvalid.Normalize(name, strvalid.NormalizeConfig{TrimSpaces: true, Lowercase: false})
}

// Expects a normalized name.
func ValidateName(name string) error {
	return strvalid.ValidateUnicode(name, nameConfig)
}
