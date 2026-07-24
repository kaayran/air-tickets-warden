package api

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date carries the API's date-only JSON representation ("2026-07-15") of the
// domain's UTC-midnight time.Time. Timestamps proper (muted_until, created_at)
// stay RFC 3339.
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Format(dateLayout) + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("date must be a %q string", dateLayout)
	}
	t, err := time.ParseInLocation(dateLayout, s[1:len(s)-1], time.UTC)
	if err != nil {
		return fmt.Errorf("invalid date %s: want %q", s, dateLayout)
	}
	d.Time = t
	return nil
}

// dateOrNil converts a nullable domain date to its API shape.
func dateOrNil(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{*t}
}

// timeOrNil converts a nullable API date back to the domain shape.
func timeOrNil(d *Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}
