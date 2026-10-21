// Package garden turns reading time into a garden: it decides which day a
// moment belongs to, counts active reading time, and draws one tree per day.
package garden

import "time"

// DayStart is when a reading day begins: reading past midnight belongs to
// the evening it started in.
const DayStart = 4 * time.Hour

const layout = "2006-01-02"

// DayTime is local midnight of the reading day t falls in.
func DayTime(t time.Time) time.Time {
	t = t.Add(-DayStart)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Key names a day the way state files store it.
func Key(day time.Time) string { return day.Format(layout) }

// DayOf is the key of t's reading day.
func DayOf(t time.Time) string { return Key(DayTime(t)) }

// ParseDay reads a key back into local midnight.
func ParseDay(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(layout, s, time.Local)
	return t, err == nil
}

// Monday is the Monday of day's week.
func Monday(day time.Time) time.Time {
	back := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -back)
}
