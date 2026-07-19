package board

import (
	"fmt"
	"time"
)

type ID string

type BoardStatus string

const (
	BoardActive BoardStatus = "Active"
	BoardClosed BoardStatus = "Closed"
	MainId      ID          = "main"
)

type Board struct {
	BoardId   ID
	BoardName string
	Status    BoardStatus
	CreatedAt time.Time
}

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
