package board

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/salandered/apex/apextime"
)

const (
	MinBoardNameRunes = 3
	MaxBoardNameRunes = 32
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

// Allowed: lowercase a-z, 0-9 and inner single hyphens; len is 3-32
func (id ID) Validate() error {
	if len(id) < 3 || len(id) > 32 {
		return fmt.Errorf("invalid board id %q: length must be in [3, 32]", string(id))
	}
	prevHyphen := false
	for i := 0; i < len(id); i++ {
		char := id[i]
		switch {
		case char >= 'a' && char <= 'z' || char >= '0' && char <= '9':
			prevHyphen = false
		case char == '-':
			if i == 0 || i == len(id)-1 {
				return fmt.Errorf("invalid board id '%q': must not start or end with '-'", string(id))
			}
			if prevHyphen {
				return fmt.Errorf("invalid board id '%q': consecutive '-' are not allowed", string(id))
			}
			prevHyphen = true
		default:
			return fmt.Errorf("invalid board id '%q': only a-z, 0-9 and '-' are allowed", string(id))
		}
	}
	return nil
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

// Normalizes and validates the caller-provided fields; fills the generated ones.
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
	return strings.TrimSpace(name)
}

// Unicode is allowed except for control chars; len is 3-32 runes.
// Note that emoji can be built from several runes.
// Expects a normalized name.
func ValidateName(name string) error {
	if name != NormalizeName(name) {
		return fmt.Errorf("invalid board name %q: must not be surrounded by spaces", name)
	}
	runes := 0
	for _, char := range name {
		if unicode.IsControl(char) {
			return fmt.Errorf("invalid board name %q: control characters are not allowed", name)
		}
		runes++
	}
	if runes < MinBoardNameRunes || runes > MaxBoardNameRunes {
		return fmt.Errorf("invalid board name %q: length must be in [%d, %d] characters",
			name, MinBoardNameRunes, MaxBoardNameRunes)
	}
	return nil
}
