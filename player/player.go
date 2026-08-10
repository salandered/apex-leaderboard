// TODO: inconsitent. Should be called profile or playerprofile
package player

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/salandered/apex/apextime"
)

const (
	MinPlayerNameLen = 3
	MaxPlayerNameLen = 32
)

type Profile struct {
	PlayerId   ID
	PlayerName string
	CreatedAt  time.Time
}

type ID string

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

// Normalizes and validates the caller-provided fields; fills the generated ones.
func NewProfile(playerName string) (*Profile, error) {
	playerName = NormalizeName(playerName)
	if err := ValidateName(playerName); err != nil {
		return nil, err
	}
	return &Profile{
		PlayerId:   GenerateID(),
		PlayerName: playerName,
		CreatedAt:  apextime.Now(),
	}, nil
}

// Trims surrounding spaces.
func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

// Allowed: a-z, A-Z, 0-9, ' ', '_', '-'; must start with a letter; len is 3-32.
// Expects a normalized name.
func ValidateName(name string) error {
	if name != NormalizeName(name) {
		return fmt.Errorf("invalid player name %q: must not be surrounded by spaces", name)
	}
	if len(name) < MinPlayerNameLen || len(name) > MaxPlayerNameLen {
		return fmt.Errorf("invalid player name %q: length must be in [%d, %d]",
			name, MinPlayerNameLen, MaxPlayerNameLen)
	}
	for i := 0; i < len(name); i++ {
		char := name[i]
		switch {
		case char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9' || char == ' ' || char == '_' || char == '-':
			if i == 0 {
				return fmt.Errorf("invalid player name %q: must start with a letter", name)
			}
		default:
			return fmt.Errorf("invalid player name %q: only a-z, A-Z, 0-9, ' ', '_' and '-' are allowed", name)
		}
	}
	return nil
}
