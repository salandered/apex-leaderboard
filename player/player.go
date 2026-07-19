package player

import (
	"fmt"
	"time"

	"github.com/google/uuid"
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
