package garden

import "time"

// Idle is the longest gap between two activities that still counts in
// full; a longer gap counts as Idle, so walking away costs little.
const Idle = 3 * time.Minute

// Credit is reading time for one day.
type Credit struct {
	Day string
	D   time.Duration
}

// Counter measures active reading time from activity events (keys,
// scrolls, clicks). The zero value is paused.
type Counter struct {
	last   time.Time
	active bool
}

// Start begins counting from now (a book opened, the terminal regained
// focus) without crediting anything.
//
// Times are compared by the wall clock (Round(0) drops the monotonic
// reading): the monotonic clock stops while a laptop sleeps, so it would
// hide a night with the lid closed from the Idle cap.
func (c *Counter) Start(now time.Time) { c.last, c.active = now.Round(0), true }

// Activity records an event and returns the time since the previous one,
// capped at Idle and split at day boundaries. While paused it only starts.
func (c *Counter) Activity(now time.Time) []Credit {
	if !c.active {
		c.Start(now)
		return nil
	}
	now = now.Round(0)
	from := c.last
	c.last = now
	if !now.After(from) {
		return nil // the clock went back, or no time passed
	}
	if now.Sub(from) > Idle {
		now = from.Add(Idle)
	}
	var out []Credit
	for from.Before(now) {
		end := DayTime(from).AddDate(0, 0, 1).Add(DayStart) // next 04:00
		if end.After(now) {
			end = now
		}
		out = append(out, Credit{DayOf(from), end.Sub(from)})
		from = end
	}
	return out
}

// Pause credits the time up to now and stops counting (focus lost).
func (c *Counter) Pause(now time.Time) []Credit {
	if !c.active {
		return nil
	}
	out := c.Activity(now)
	c.active = false
	return out
}
