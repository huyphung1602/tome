package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/state"
)

// fakeClock returns a clock the test moves by hand.
func fakeClock(start string) (func() time.Time, func(time.Duration)) {
	now, _ := time.ParseInLocation("2006-01-02 15:04:05", start, time.Local)
	return func() time.Time { return now }, func(d time.Duration) { now = now.Add(d) }
}

func readingModel(t *testing.T) (*Model, *state.Position, func(time.Duration)) {
	t.Helper()
	saved := &state.Position{}
	m := New(testBook(), state.Position{Reading: map[string]int{"2026-10-21": 600}}, func(p state.Position) {
		saved.Reading = state.MergeReading(saved.Reading, p.ReadingAdd)
	})
	clock, advance := fakeClock("2026-10-21 20:00:00")
	m.SetClock(clock)
	m.SetGarden(map[string]map[string]int{"2026-10-21": {"Other": 300}}, 30, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, saved, advance
}

func TestReadingCountsWithoutFocusEvents(t *testing.T) {
	m, saved, advance := readingModel(t)
	advance(40 * time.Second)
	keys(m, runes("j"))
	advance(10 * time.Minute) // walked away: 3 minutes count
	keys(m, runes("j"))
	m.persist()
	if got := saved.Reading["2026-10-21"]; got != 40+180 {
		t.Errorf("saved %d seconds, want 220", got)
	}
	m.persist() // nothing new: nothing added twice
	if got := saved.Reading["2026-10-21"]; got != 220 {
		t.Errorf("second save added again: %d", got)
	}
}

func TestReadingPausesOnBlur(t *testing.T) {
	m, saved, advance := readingModel(t)
	advance(time.Minute)
	m.Update(tea.BlurMsg{})
	advance(time.Hour)
	keys(m, runes("j")) // a key while blurred only restarts the count
	m.Update(tea.FocusMsg{})
	advance(30 * time.Second)
	keys(m, runes("j"))
	m.persist()
	if got := saved.Reading["2026-10-21"]; got != 90 {
		t.Errorf("saved %d seconds, want 90", got)
	}
}

func TestReadingWebScrollCounts(t *testing.T) {
	m, saved, advance := readingModel(t)
	advance(20 * time.Second)
	m.Update(WebPos{Chapter: 1, Block: 0})
	m.persist()
	if got := saved.Reading["2026-10-21"]; got != 20 {
		t.Errorf("saved %d", got)
	}
}

func TestStatusShowsToday(t *testing.T) {
	m, _, advance := readingModel(t)
	advance(2 * time.Minute)
	keys(m, runes("j"))
	// 600s saved for this book + 300s other book + 120s now = 17m
	if s := m.status(); !strings.Contains(s, "today 17m/30m") {
		t.Errorf("status = %q", s)
	}
	if got := m.gardenBooks()["2026-10-21"]; got["T"] != 720 || got["Other"] != 300 {
		t.Errorf("garden books = %v", got)
	}
}

// The minute tick hands over only reading time, and only when there is
// some, so an idle window never rewrites another window's position.
func TestTickSavesOnlyNewReading(t *testing.T) {
	var saves []state.Position
	m := New(testBook(), state.Position{}, func(p state.Position) { saves = append(saves, p) })
	clock, advance := fakeClock("2026-10-21 20:00:00")
	m.SetClock(clock)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	advance(1500 * time.Millisecond)
	keys(m, runes("j"))
	m.Update(readTickMsg{})
	if len(saves) != 1 || !saves[0].ReadingOnly || saves[0].ReadingAdd["2026-10-21"] != 1 {
		t.Fatalf("first tick saved %+v", saves)
	}
	m.Update(readTickMsg{}) // half a second left over: nothing to hand over
	if len(saves) != 1 {
		t.Errorf("idle tick saved again: %+v", saves[1:])
	}
}
