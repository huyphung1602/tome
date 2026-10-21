package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/garden"
)

// Reading time for the garden: any key, click, scroll or browser scroll
// while a book is open is activity, and the counter turns the gaps between
// activities into time (each gap capped at garden.Idle).

type readTickMsg struct{}

// readTick saves the time counted so far once a minute, so a crash loses
// at most a minute.
func readTick() tea.Cmd {
	return tea.Tick(time.Minute, func(time.Time) tea.Msg { return readTickMsg{} })
}

// SetClock replaces the clock (tests) and restarts the count from it.
func (m *Model) SetClock(now func() time.Time) {
	m.clock = now
	m.counter.Start(now())
}

// SetGarden gives the reader the other books' reading time, the daily goal
// in minutes, and how to save a new goal.
func (m *Model) SetGarden(others map[string]map[string]int, goal int, setGoal func(int)) {
	m.others, m.goal, m.setGoal = others, goal, setGoal
}

// track feeds an incoming message to the counter.
func (m *Model) track(msg tea.Msg) {
	now := m.clock()
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg, WebPos:
		m.credit(m.counter.Activity(now))
	case tea.FocusMsg:
		m.counter.Start(now)
	case tea.BlurMsg:
		m.credit(m.counter.Pause(now))
	}
}

func (m *Model) credit(cs []garden.Credit) {
	for _, c := range cs {
		m.mine[c.Day] += c.D
		m.unsaved[c.Day] += c.D
	}
}

// drainReading hands over whole seconds not yet saved, keeping the rest.
func (m *Model) drainReading() map[string]int {
	var out map[string]int
	for day, d := range m.unsaved {
		if s := int(d / time.Second); s > 0 {
			if out == nil {
				out = map[string]int{}
			}
			out[day] = s
			m.unsaved[day] = d - time.Duration(s)*time.Second
		}
	}
	return out
}

// todaySecs is today's reading time across every book.
func (m *Model) todaySecs() int {
	today := garden.DayOf(m.clock())
	s := int(m.mine[today] / time.Second)
	for _, secs := range m.others[today] {
		s += secs
	}
	return s
}

// gardenBooks is every book's time per day, this one live.
func (m *Model) gardenBooks() map[string]map[string]int {
	out := map[string]map[string]int{}
	for day, books := range m.others {
		out[day] = map[string]int{}
		for t, s := range books {
			out[day][t] = s
		}
	}
	for day, d := range m.mine {
		if out[day] == nil {
			out[day] = map[string]int{}
		}
		out[day][m.book.Title] += int(d / time.Second)
	}
	return out
}

func (m *Model) openGarden() {
	goal := m.goal
	if goal == 0 {
		goal = 30
	}
	setGoal := func(g int) {
		m.goal = g
		if m.setGoal != nil {
			m.setGoal(g)
		}
	}
	m.garden = newGarden(m.gardenBooks(), goal, setGoal, m.clock())
	m.overlay = overlayGarden
}
