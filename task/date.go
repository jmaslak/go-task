package task

import (
	"fmt"
	"time"
)

// DateLayout is the wire format for dates in task files.
const DateLayout = "2006-01-02"

// Date is a calendar date with no time of day and no time zone. The zero
// value means "not set".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseDate parses a date in YYYY-MM-DD form.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(DateLayout, s, time.Local)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD format", s)
	}
	return DateOf(t), nil
}

// DateOf returns the calendar date on which t falls.
func DateOf(t time.Time) Date {
	year, month, day := t.Date()
	return Date{Year: year, Month: month, Day: day}
}

// Today returns the current local date.
func Today() Date {
	return DateOf(time.Now())
}

// IsZero reports whether the date is unset.
func (d Date) IsZero() bool {
	return d == Date{}
}

// String renders the date in YYYY-MM-DD form, the format used both in task
// files and when prompting the user.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Time().Format(DateLayout)
}

// Time returns midnight local time on the date.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.Local)
}

// AddDays returns the date days later, where days may be negative.
func (d Date) AddDays(days int) Date {
	return DateOf(d.Time().AddDate(0, 0, days))
}

// Before reports whether d falls before other.
func (d Date) Before(other Date) bool {
	return d.Time().Before(other.Time())
}

// After reports whether d falls after other.
func (d Date) After(other Date) bool {
	return other.Before(d)
}

// DayCount returns the Modified Julian Day of the date. It is the same
// day numbering the Raku implementation used, so tasks with a display
// frequency come up on the same days as they did before.
func (d Date) DayCount() int64 {
	const unixEpochMJD = 40587
	utc := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	return utc.Unix()/86400 + unixEpochMJD
}

// Pretty renders the date the way task headers show it: the end of the day,
// in the same shape as a Unix ctime string.
func (d Date) Pretty() string {
	t := d.Time()
	return fmt.Sprintf("%s %s %2d 23:59:59 %d",
		t.Format("Mon"), t.Format("Jan"), d.Day, d.Year)
}
