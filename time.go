package wordpress

import (
	"fmt"
	"net/url"
	"time"
)

// defaultLocation is the time.Location used when decoding timestamps from WordPress.
var defaultLocation = time.UTC

// SetDefaultLocation configure default location
func SetDefaultLocation(loc *time.Location) {
	defaultLocation = loc
}

// Time is a wrapper around time.Time with custom JSON marshal/unmarshal functions for the WordPress specific timestamp formats.
type Time struct {
	time.Time
}

// TimeGMT is a same kind of time.Time wrapper with Time, but is considered as a time in GMT.
type TimeGMT struct {
	time.Time
}

const (
	// TimeLayout is the layout string for a timestamp without timezone information like 2017-12-25T09:54:42
	TimeLayout = "2006-01-02T15:04:05"

	// TimeWithZoneLayout is the layout string for a timestamp with timezone information like 2017-09-24T13:28:06+00:00.
	TimeWithZoneLayout = "2006-01-02T15:04:05-07:00"

	// TimeWithZoneLayout is the layout string for a timestamp which is simple for human like 2017-12-25 09:54:42
	SimpleTimeLayout = "2006-01-02 15:04:05"
)

func unmarshalTimeJSON(t *time.Time, b []byte, loc *time.Location) error {
	if b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}

	parsedTime, err := time.Parse(TimeWithZoneLayout, string(b))
	if err == nil {
		*t = parsedTime.In(loc)
		return nil
	}

	parsedTime, err = time.ParseInLocation(TimeLayout, string(b), loc)
	if err == nil {
		*t = parsedTime
		return nil
	}

	parsedTime, err = time.ParseInLocation(SimpleTimeLayout, string(b), loc)
	if err == nil {
		*t = parsedTime
		return nil
	}

	return fmt.Errorf("cannot parse \"%s\" as any of WordPress time layouts: \"%s\", \"%s\", \"%s\"", b, TimeWithZoneLayout, TimeLayout, SimpleTimeLayout)
}

func marshalTimeJSON(t *time.Time) ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, t.Format(TimeLayout))), nil
}

func encodeValues(t *time.Time, key string, uv *url.Values, loc *time.Location) error {
	uv.Add(key, t.In(loc).Format(TimeLayout))
	return nil
}

// NewTime returns a WordPress Time
func NewTime(t *time.Time) *Time {
	return &Time{
		Time: *t,
	}
}

// UnmarshalJSON unmarshals the timestamp with one of the WordPress specific formats.
func (t *Time) UnmarshalJSON(b []byte) error {
	return unmarshalTimeJSON(&t.Time, b, defaultLocation)
}

// MarshalJSON returns a WordPress formatted timestamp.
func (t *Time) MarshalJSON() ([]byte, error) {
	return marshalTimeJSON(&t.Time)
}

// EncodeValues encodes a value and add it to url query
func (t *Time) EncodeValues(key string, uv *url.Values) error {
	return encodeValues(&t.Time, key, uv, defaultLocation)
}

// UnmarshalJSON unmarshals the timestamp with one of the WordPress specific formats.
func (t *TimeGMT) UnmarshalJSON(b []byte) error {
	return unmarshalTimeJSON(&t.Time, b, time.UTC)
}

// MarshalJSON returns a WordPress formatted timestamp.
func (t *TimeGMT) MarshalJSON() ([]byte, error) {
	return marshalTimeJSON(&t.Time)
}
