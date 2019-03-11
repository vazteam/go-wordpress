package wordpress

import (
	"fmt"
	"time"
)

// Location is the time.Location used when decoding timestamps from WordPress.
var Location = time.UTC

// Time is a wrapper around time.Time with custom JSON marshal/unmarshal functions for the WordPress specific timestamp formats.
type Time struct {
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

// UnmarshalJSON unmarshals the timestamp with one of the WordPress specific formats.
func (t *Time) UnmarshalJSON(b []byte) error {
	if b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}

	parsedTime, err := time.Parse(TimeWithZoneLayout, string(b))
	if err == nil {
		t.Time = parsedTime
		return nil
	}

	parsedTime, err = time.ParseInLocation(TimeLayout, string(b), Location)
	if err == nil {
		t.Time = parsedTime
		return nil
	}

	parsedTime, err = time.ParseInLocation(SimpleTimeLayout, string(b), Location)
	if err == nil {
		t.Time = parsedTime
		return nil
	}

	return fmt.Errorf("cannot parse \"%s\" as any of WordPress time layouts: \"%s\", \"%s\", \"%s\"", b, TimeWithZoneLayout, TimeLayout, SimpleTimeLayout)
}

// MarshalJSON returns a WordPress formatted timestamp.
func (t *Time) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, t.Time.Format(TimeLayout))), nil
}
