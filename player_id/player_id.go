package playerid

import (
	"fmt"

	"github.com/google/uuid"
)

type PlayerId string

func (p PlayerId) String() string {
	return string(p)
}

func (p PlayerId) Validate() error {
	if _, err := uuid.Parse(string(p)); err != nil {
		return fmt.Errorf("invalid player id %q: %w", string(p), err)
	}
	return nil
}

func GeneratePlayerId() PlayerId {
	return PlayerId(uuid.New().String())
}
