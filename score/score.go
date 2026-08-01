package score

import "fmt"

// Scores live in Redis ZSETs, whose scores are IEEE-754 doubles.
// Max (1e13) is chosen so a stored score and any intermediate sum (at most 2*Max)
// stay under the 2^53 (~9.e15) (float64 integer boundary).
// It also allows a unix millisecond timestamp, a possible client use case for a time-based ranking.
const (
	Max int64 = 10_000_000_000_000 // 1e13
	Min int64 = -Max
)

func Validate(v int64) error {
	if v < Min || v > Max {
		return fmt.Errorf("invalid score %d: must be in [-1e13, 1e13]", v)
	}
	return nil
}
