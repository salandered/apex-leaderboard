package player

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MinPlayerNameLen = 3
	MaxPlayerNameLen = 32
)

type ID string

type Profile struct {
	PlayerId   ID
	PlayerName string
	CreatedAt  time.Time
}

func (id ID) String() string {
	return string(id)
}

func (id ID) Validate() error {
	if _, err := uuid.Parse(string(id)); err != nil {
		return fmt.Errorf("invalid player id %q: %w", string(id), err)
	}
	return nil
}

func GenerateID() ID {
	return ID(uuid.New().String())
}

// Allowed: a-z, A-Z, 0-9, ' ', '_', '-'; must start with a letter; len is 3-32
// Trims surrounding spaces.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) < MinPlayerNameLen || len(name) > MaxPlayerNameLen {
		return "", fmt.Errorf("invalid player name %q: length must be in [%d, %d]",
			name, MinPlayerNameLen, MaxPlayerNameLen)
	}
	for i := 0; i < len(name); i++ {
		char := name[i]
		switch {
		case char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9' || char == ' ' || char == '_' || char == '-':
			if i == 0 {
				return "", fmt.Errorf("invalid player name %q: must start with a letter", name)
			}
		default:
			return "", fmt.Errorf("invalid player name %q: only a-z, A-Z, 0-9, ' ', '_' and '-' are allowed", name)
		}
	}
	return name, nil
}
