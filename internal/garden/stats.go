package garden

import "time"

// Days maps a day key to seconds read that day, across books.
type Days map[string]int

// read reports whether a day counts toward a streak: a minute or more.
func (d Days) read(day time.Time) bool { return d[Key(day)] >= 60 }

// Streak counts days read in a row up to today. Until today is read, it
// counts up to yesterday, so it doesn't reset in the morning.
func Streak(d Days, today time.Time) int {
	day := today
	if !d.read(day) {
		day = day.AddDate(0, 0, -1)
	}
	n := 0
	for d.read(day) {
		n++
		day = day.AddDate(0, 0, -1)
	}
	return n
}

// MonthStats counts the days read in a month and its longest run, up to today.
func MonthStats(d Days, year int, month time.Month, today time.Time) (daysRead, longest int) {
	run := 0
	for day := time.Date(year, month, 1, 0, 0, 0, 0, today.Location()); day.Month() == month && !day.After(today); day = day.AddDate(0, 0, 1) {
		if d.read(day) {
			daysRead++
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return daysRead, longest
}
