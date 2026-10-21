package web

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/huyphung1602/tome/internal/garden"
)

// Garden is the reading time tome sends to the page: seconds per day per
// book title, the daily goal in minutes, and today's day key.
type Garden struct {
	Books map[string]map[string]int
	Goal  int
	Today string
}

// The page only draws; levels, weeks, months and streaks are worked out
// here with the same code as the terminal garden.
type gardenView struct {
	Summary string        `json:"summary"`
	Today   string        `json:"today"` // for the status line, like tome's: "today 12m/30m"
	Goal    int           `json:"goal"`
	Months  []gardenMonth `json:"months"`
	Legend  []gardenLevel `json:"legend"`
	Empty   bool          `json:"empty"`
}

type gardenMonth struct {
	Name  string        `json:"name"`
	Stats string        `json:"stats"`
	Weeks [][]gardenDay `json:"weeks"`
}

type gardenDay struct {
	Key    string `json:"key"`
	Label  string `json:"label"` // "Tue 20"
	Mins   string `json:"mins"`  // "52m", "" under a minute
	Level  int    `json:"level"`
	Future bool   `json:"future,omitempty"`
	Today  bool   `json:"today,omitempty"`
	Detail string `json:"detail"` // "Tue 20 · 52m · A 40m, B 12m"
}

type gardenLevel struct {
	Level int    `json:"level"`
	Label string `json:"label"`
}

func buildGarden(g Garden) gardenView {
	today, ok := garden.ParseDay(g.Today)
	if !ok {
		today = garden.DayTime(time.Now())
	}
	goal := g.Goal
	if goal <= 0 {
		goal = 30
	}
	days := garden.Days{}
	for day, bs := range g.Books {
		for _, s := range bs {
			days[day] += s
		}
	}
	v := gardenView{Goal: goal, Empty: len(days) == 0}
	v.Today = fmt.Sprintf("today %s/%dm", garden.FormatMinutes(days[garden.Key(today)]), goal)
	v.Summary = fmt.Sprintf("today %s of %dm", garden.FormatMinutes(days[garden.Key(today)]), goal)
	if n := garden.Streak(days, today); n > 0 {
		v.Summary = fmt.Sprintf("%d days in a row · ", n) + v.Summary
	}
	for i, l := range garden.Labels(goal) {
		v.Legend = append(v.Legend, gardenLevel{i + 1, l})
	}

	first := garden.Monday(today)
	for k := range days {
		// a day before 2000 is a clock that was wrong, not a reader
		if d, ok := garden.ParseDay(k); ok && d.Before(first) && k >= "2000-01-01" {
			first = garden.Monday(d)
		}
	}
	for w := first; !w.After(today); w = w.AddDate(0, 0, 7) {
		// a week belongs to the month its Sunday is in, as in the terminal
		sun := w.AddDate(0, 0, 6)
		if len(v.Months) == 0 || v.Months[len(v.Months)-1].Name != monthName(sun, today) {
			read, longest := garden.MonthStats(days, sun.Year(), sun.Month(), today)
			v.Months = append(v.Months, gardenMonth{
				Name:  monthName(sun, today),
				Stats: fmt.Sprintf("%d days read · longest streak %d", read, longest),
			})
		}
		week := make([]gardenDay, 7)
		for i := range week {
			d := w.AddDate(0, 0, i)
			key := garden.Key(d)
			secs := days[key]
			day := gardenDay{Key: key, Label: d.Format("Mon 2"), Level: garden.Level(secs, goal),
				Future: d.After(today), Today: d.Equal(today), Detail: dayDetail(d, today, g.Books[key])}
			if secs >= 60 {
				day.Mins = garden.FormatMinutes(secs)
			}
			week[i] = day
		}
		m := &v.Months[len(v.Months)-1]
		m.Weeks = append(m.Weeks, week)
	}
	return v
}

func monthName(d, today time.Time) string {
	if d.Year() != today.Year() {
		return fmt.Sprint(d.Month(), " ", d.Year())
	}
	return d.Month().String()
}

// dayDetail is the selected day's line: its time and the books read.
func dayDetail(d, today time.Time, books map[string]int) string {
	label := d.Format("Mon 2")
	type bt struct {
		title string
		secs  int
	}
	var list []bt
	total := 0
	for t, s := range books {
		total += s
		if s >= 60 {
			list = append(list, bt{t, s})
		}
	}
	if total < 60 {
		if d.Equal(today) {
			return label + " · nothing read yet"
		}
		return label + " · nothing read"
	}
	slices.SortFunc(list, func(a, b bt) int { return cmp.Or(cmp.Compare(b.secs, a.secs), cmp.Compare(a.title, b.title)) })
	s := label + " · " + garden.FormatMinutes(total) + " · "
	for i, b := range list {
		if i > 0 {
			s += ", "
		}
		s += b.title + " " + garden.FormatMinutes(b.secs)
	}
	return s
}

// SetGarden tells open pages the reading time to draw.
func (s *Server) SetGarden(g Garden) {
	data, _ := json.Marshal(buildGarden(g))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.garden = string(data)
	s.broadcast("garden", s.garden)
}

// SetOnGoal is called when the page changes the daily goal.
func (s *Server) SetOnGoal(fn func(minutes int)) { s.mu.Lock(); s.onGoal = fn; s.mu.Unlock() }

// SetOnActive is called when the reader does something on the page, so
// reading there counts even without scrolling.
func (s *Server) SetOnActive(fn func()) { s.mu.Lock(); s.onActive = fn; s.mu.Unlock() }

// gardenGrids is the trees' pixel art and colors for the page.
func gardenGrids() map[string]any {
	grids := make([][10]string, garden.MaxLevel+1)
	for i := range grids {
		grids[i] = garden.Grid(i)
	}
	return map[string]any{"grids": grids}
}
