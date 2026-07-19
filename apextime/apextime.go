package apextime

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// millisecond precision, UTC
const layout = "2006-01-02T15:04:05.000Z07:00"

// format t as RFC3339Milli + Z in UTC
func Format(t time.Time) string {
	// .UTC() guarantees the trailing "Z" (not a numeric offset)
	return t.UTC().Format(layout)
}

// canonical "current time"
func Now() time.Time {
	return time.Now().UTC()
}

// any valid RFC 3339 to UTC
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// from Redis stream entry ID ("<unix_millis>-<seq>") to UTC
func FromStreamID(id string) (time.Time, error) {
	msStr, _, ok := strings.Cut(id, "-")
	if !ok {
		return time.Time{}, errors.New("apextime: malformed stream id " + id)
	}
	ms, err := strconv.ParseInt(msStr, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms).UTC(), nil
}

// Formats t using the named IANA zone ("Europe/Berlin", "America/New_York").
// Debugging/display only.
func InZone(t time.Time, zone string) (string, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return t.In(loc).Format(layout), nil
}

// marshals as RFC3339Milli+Z. Used in API DTOs
type Time struct{ time.Time }

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + Format(t.Time) + `"`), nil
}

func (t *Time) UnmarshalJSON(b []byte) error {
	parsed, err := Parse(strings.Trim(string(b), `"`))
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// func TestFormatParseRoundTrip(t *testing.T) {
//     for _, in := range []string{
//         "2026-01-17T12:30:00.000Z",
//         "2026-01-17T12:30:00.123Z",
//         "2026-01-17T12:30:00.500Z",
//     } {
//         parsed, err := Parse(in)
//         if err != nil { t.Fatal(err) }
//         if got := Format(parsed); got != in {
//             t.Errorf("round trip: %s -> %s", in, got)
//         }
//     }
// }
