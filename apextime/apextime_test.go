package apextime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatRendersMillisecondsInUTC(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			"utc is rendered as is",
			time.Date(2026, 1, 17, 12, 30, 0, 0, time.UTC),
			"2026-01-17T12:30:00.000Z",
		},
		{
			"milliseconds are kept",
			time.Date(2026, 1, 17, 12, 30, 0, 123_000_000, time.UTC),
			"2026-01-17T12:30:00.123Z",
		},
		{
			// Format truncates, it does not round
			"sub millisecond precision is truncated",
			time.Date(2026, 1, 17, 12, 30, 0, 123_999_999, time.UTC),
			"2026-01-17T12:30:00.123Z",
		},
		{
			// the whole point of the .UTC() call in Format
			"an offset zone is converted to utc, never a numeric offset",
			time.Date(2026, 1, 17, 14, 30, 0, 0, time.FixedZone("CET", 2*60*60)),
			"2026-01-17T12:30:00.000Z",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, Format(c.in))
		})
	}
}

func TestParseConvertsToUTC(t *testing.T) {
	for _, in := range []string{
		"2026-01-17T12:30:00.000Z",
		"2026-01-17T14:30:00+02:00",
		"2026-01-17T07:30:00-05:00", // all three the same
	} {
		got, err := Parse(in)
		require.NoErrorf(t, err, "input %q", in)
		require.Equalf(t, time.UTC, got.Location(), "input %q", in)
		require.Equalf(t, "2026-01-17T12:30:00.000Z", Format(got), "input %q", in)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	for _, rawStr := range []string{
		"",
		"2026-01-17",               // date only
		"12:30:00",                 // time only
		"2026-01-17 12:30:00Z",     // space instead of T
		"2026-01-17T12:30:00",      // no zone
		"2026-13-17T12:30:00.000Z", // month 13
	} {
		_, err := Parse(rawStr)
		require.Errorf(t, err, "input %q should not parse", rawStr)
	}
}

func TestParseDateReturnsUTCStartOfDay(t *testing.T) {
	got, err := ParseDate("2026-07-18")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC), got)
}

func TestParseDateRejectsTimestamp(t *testing.T) {
	_, err := ParseDate("2026-07-18T00:00:00Z")
	require.Error(t, err)
}

func TestFormatParseRoundTrip(t *testing.T) {
	for _, rawStr := range []string{
		"2026-01-17T12:30:00.000Z",
		"2026-01-17T12:30:00.123Z",
		"2026-01-17T12:30:00.500Z",
		"1970-01-01T00:00:00.000Z",
	} {
		timestamp, err := Parse(rawStr)
		require.NoError(t, err)
		require.Equal(t, rawStr, Format(timestamp))
	}
}

func TestFromStreamID(t *testing.T) {
	// redis stream ids are "<unix_millis>-<sequence>"
	timestamp, err := FromStreamID("1768652200123-0")
	require.NoError(t, err)
	require.Equal(t, int64(1768652200123), timestamp.UnixMilli())
	require.Equal(t, time.UTC, timestamp.Location())

	// the sequence part is ignored
	sameTimestamp, err := FromStreamID("1768652200123-7")
	require.NoError(t, err)
	require.True(t, timestamp.Equal(sameTimestamp))
}

func TestFromStreamIDRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{
		"",              // empty
		"1768652200123", // no separator
		"abc-0",         // non numeric millis
		"-0",            // missing millis
	} {
		_, err := FromStreamID(in)
		require.Errorf(t, err, "input %q should not parse", in)
	}
}

// Cut splits on the FIRST dash and the remainder is never validated, so anything
// with a numeric prefix is accepted. Redis always supplies well formed ids.
func TestFromStreamIDAcceptsAnyNumericPrefix(t *testing.T) {
	got, err := FromStreamID("2026-01-17T12:30")
	require.NoError(t, err)
	require.Equal(t, int64(2026), got.UnixMilli()) // read as 2026 millis past the epoch
}

func TestInZoneRendersWallClockOfTheNamedZone(t *testing.T) {
	instant := time.Date(2026, 1, 17, 12, 30, 0, 0, time.UTC)

	// winter: Berlin is UTC+1
	got, err := InZone(instant, "Europe/Berlin")
	require.NoError(t, err)
	require.Equal(t, "2026-01-17T13:30:00.000+01:00", got)

	got, err = InZone(instant, "UTC")
	require.NoError(t, err)
	require.Equal(t, "2026-01-17T12:30:00.000Z", got)
}

func TestInZoneUnknownZoneReturnsError(t *testing.T) {
	_, err := InZone(Now(), "Mars/Olympus_Mons")
	require.Error(t, err)
}

func TestTimeMarshalsAsRFC3339Milli(t *testing.T) {
	payload := struct {
		CreatedAt Time `json:"created_at"`
	}{
		CreatedAt: Time{time.Date(2026, 1, 17, 12, 30, 0, 123_000_000, time.UTC)},
	}

	got, err := json.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, `{"created_at":"2026-01-17T12:30:00.123Z"}`, string(got))
}

func TestTimeUnmarshalsFromRFC3339(t *testing.T) {
	var payload struct {
		CreatedAt Time `json:"created_at"`
	}

	// an offset in the wire format lands as UTC
	require.NoError(t, json.Unmarshal(
		[]byte(`{"created_at":"2026-01-17T14:30:00+02:00"}`), &payload))
	require.Equal(t, "2026-01-17T12:30:00.000Z", Format(payload.CreatedAt.Time))
	require.Equal(t, time.UTC, payload.CreatedAt.Location())
}

func TestTimeUnmarshalRejectsMalformedInput(t *testing.T) {
	var payload struct {
		CreatedAt Time `json:"created_at"`
	}
	err := json.Unmarshal([]byte(`{"created_at":"not a timestamp"}`), &payload)
	require.Error(t, err)
}

func TestTimeJsonRoundTrip(t *testing.T) {
	original := Time{time.Date(2026, 1, 17, 12, 30, 0, 500_000_000, time.UTC)}

	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Time
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.True(t, original.Equal(decoded.Time))
}

func TestNowIsUTC(t *testing.T) {
	require.Equal(t, time.UTC, Now().Location())
}
