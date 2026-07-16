package player

import (
	"fmt"

	"github.com/google/uuid"
)

type ID string

func (p ID) String() string {
	return string(p)
}

func (p ID) Validate() error {
	if _, err := uuid.Parse(string(p)); err != nil {
		return fmt.Errorf("invalid player id %q: %w", string(p), err)
	}
	return nil
}

func GenerateID() ID {
	return ID(uuid.New().String())
}

// TODO: DateAdded and other
type Profile struct {
	PlayerId   ID
	PlayerName string
}

// ScoreEntry is a ranked leaderboard row for future reads like Top-N
type ScoreEntry struct {
	PlayerId   ID
	PlayerName string
	Score      float64
	Rank       int64
}
