// TODO: inconsitent. Should be called profile or playerprofile
package player

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/salandered/apex/apextime"
	"github.com/salandered/strvalid"
)

// a-z, A-Z, 0-9, ' ' and inner '_', '-'; must start with a letter; len is 3-32.
var nameConfig = strvalid.Config{
	Subject: "player name",
	MinLen:  3,
	MaxLen:  32,

	Upper:      true,
	Digits:     true,
	Space:      true,
	Underscore: strvalid.SepInner,
	Dash:       strvalid.SepInner,

	AllowRepeatSep: true,

	LeadingLetter: true,
	EchoValue:     true,
}

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
	return strvalid.Normalize(name, strvalid.NormalizeConfig{TrimSpaces: true, Lowercase: false})
}

// Expects a normalized name.
func ValidateName(name string) error {
	return strvalid.Validate(name, nameConfig)
}
