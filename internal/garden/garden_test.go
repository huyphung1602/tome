package garden

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func sum(cs []Credit) map[string]time.Duration {
	out := map[string]time.Duration{}
	for _, c := range cs {
		out[c.Day] += c.D
	}
	return out
}

func TestDayStartsAtFour(t *testing.T) {
	for in, want := range map[string]string{
		"2026-10-21 03:59:00": "2026-10-20",
		"2026-10-21 04:00:00": "2026-10-21",
		"2026-10-21 23:30:00": "2026-10-21",
	} {
		if got := DayOf(at(in)); got != want {
			t.Errorf("DayOf(%s) = %s, want %s", in, got, want)
		}
	}
	if got := Key(Monday(at("2026-10-25 12:00:00"))); got != "2026-10-19" { // a Sunday
		t.Errorf("Monday = %s", got)
	}
	if d, ok := ParseDay("2026-10-21"); !ok || Key(d) != "2026-10-21" {
		t.Errorf("ParseDay = %v %v", d, ok)
	}
}

func TestCounterCapsIdleGaps(t *testing.T) {
	var c Counter
	c.Start(at("2026-10-21 20:00:00"))
	got := sum(c.Activity(at("2026-10-21 20:00:40")))
	got2 := sum(c.Activity(at("2026-10-21 20:10:40"))) // 10 minutes away: only 3 count
	if got["2026-10-21"] != 40*time.Second || got2["2026-10-21"] != Idle {
		t.Errorf("got %v then %v", got, got2)
	}
}

func TestCounterSplitsAcrossFourAM(t *testing.T) {
	var c Counter
	c.Start(at("2026-10-22 03:59:00"))
	got := sum(c.Activity(at("2026-10-22 04:01:00")))
	if got["2026-10-21"] != time.Minute || got["2026-10-22"] != time.Minute {
		t.Errorf("got %v", got)
	}
}

func TestCounterPauseAndResume(t *testing.T) {
	var c Counter
	c.Start(at("2026-10-21 20:00:00"))
	paused := sum(c.Pause(at("2026-10-21 20:01:00")))
	if paused["2026-10-21"] != time.Minute {
		t.Errorf("pause credited %v", paused)
	}
	if cs := c.Activity(at("2026-10-21 20:30:00")); len(cs) != 0 { // first event after a pause only restarts
		t.Errorf("activity while paused credited %v", cs)
	}
	if got := sum(c.Activity(at("2026-10-21 20:30:20"))); got["2026-10-21"] != 20*time.Second {
		t.Errorf("after resume got %v", got)
	}
}

func TestCounterIgnoresClockGoingBack(t *testing.T) {
	var c Counter
	c.Start(at("2026-10-21 20:00:00"))
	if cs := c.Activity(at("2026-10-21 19:00:00")); len(cs) != 0 {
		t.Errorf("backwards credited %v", cs)
	}
	if got := sum(c.Activity(at("2026-10-21 19:00:30"))); got["2026-10-21"] != 30*time.Second {
		t.Errorf("after the jump got %v", got)
	}
}

// Monotonic clock readings stop during sleep, so a lid closed across 04:00
// must be measured by the wall clock, or the whole night counts.
func TestCounterUsesWallClock(t *testing.T) {
	var c Counter
	c.Start(time.Now())
	if c.last != c.last.Round(0) {
		t.Error("Start kept the monotonic reading")
	}
	c.Activity(time.Now())
	if c.last != c.last.Round(0) {
		t.Error("Activity kept the monotonic reading")
	}
}
