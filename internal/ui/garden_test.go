package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func gardenAt(t *testing.T, books map[string]map[string]int) (*gardenScreen, *int) {
	t.Helper()
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-21 20:00", time.Local) // a Wednesday
	goal := 30
	return newGarden(books, goal, func(g int) { goal = g }, now), &goal
}

func plain(s string) string { return ansi.Strip(s) }

func TestGardenScreen(t *testing.T) {
	g, _ := gardenAt(t, map[string]map[string]int{
		"2026-10-19": {"A": 34 * 60},
		"2026-10-20": {"A": 52 * 60, "B": 20 * 60},
		"2026-09-22": {"B": 10 * 60}, // a week whose Sunday is in September
	})
	out := plain(g.view(100, 60))
	for _, want := range []string{
		"October · 2 days read · longest streak 2",
		"September · 1 days read",
		"Mon 19", "34m", "Tue 20", "1h12", "Wed 21",
		"<15m", "15–30m", "2h+", "goal 30m/day",
		"Wed 21 · nothing read yet", // today is selected
	} {
		if !strings.Contains(out, want) {
			t.Errorf("garden lacks %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(g.view(100, 60), "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d wide: %q", w, plain(line))
		}
	}
}

func TestGardenSelectAndGoal(t *testing.T) {
	g, goal := gardenAt(t, map[string]map[string]int{"2026-10-20": {"A": 52 * 60, "B": 20 * 60}})
	g.key("left")
	if out := plain(g.view(100, 60)); !strings.Contains(out, "Tue 20 · 1h12 · A 52m, B 20m") {
		t.Errorf("selected day status missing:\n%s", out)
	}
	g.key("right")
	g.key("right") // can't go past today
	if !strings.Contains(plain(g.view(100, 60)), "Wed 21 ·") {
		t.Error("selection went past today")
	}
	g.key("+")
	if *goal != 35 || g.goal != 35 {
		t.Errorf("goal = %d", *goal)
	}
	for i := 0; i < 30; i++ {
		g.key("-")
	}
	if g.goal != 5 {
		t.Errorf("goal floor = %d", g.goal)
	}
	if !g.key("esc") {
		t.Error("esc should close")
	}
}

func TestGardenEmpty(t *testing.T) {
	g, _ := gardenAt(t, nil)
	if out := plain(g.view(100, 40)); !strings.Contains(out, "read a book and your first tree grows here") {
		t.Errorf("no empty message:\n%s", out)
	}
}

func TestReaderOpensGarden(t *testing.T) {
	m, _, _ := readingModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	if m.overlay != overlayGarden || !strings.Contains(plain(m.View()), "goal 30m/day") {
		t.Fatalf("ctrl+g didn't open the garden:\n%s", plain(m.View()))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.overlay != overlayNone {
		t.Error("esc didn't close the garden")
	}
}

func TestGardenFitsEightyColumns(t *testing.T) {
	g, _ := gardenAt(t, map[string]map[string]int{"2026-10-20": {"A": 60}})
	out := g.view(80, 40)
	for _, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line %d wide: %q", w, plain(line))
		}
	}
	if !strings.Contains(plain(out), "goal 30m/day · + −") {
		t.Error("goal hint missing")
	}
}

func TestGardenClickOnlyVisibleDays(t *testing.T) {
	books := map[string]map[string]int{}
	for d := 1; d <= 21; d++ {
		books[fmt.Sprintf("2026-10-%02d", d)] = map[string]int{"A": 600}
	}
	books["2026-06-01"] = map[string]int{"A": 600} // months of weeks to scroll
	g, _ := gardenAt(t, books)
	g.view(100, 40)
	g.top = 0 // scrolled to the top: this week is far below
	g.view(100, 40)
	// a click on the legend's trees, below the visible weeks
	g.mouse(tea.MouseMsg{X: g.left + 3, Y: 40 - 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !g.sel.Equal(g.today) {
		t.Errorf("click outside the weeks selected %v", g.sel)
	}
}

func TestGardenIgnoresBogusDays(t *testing.T) {
	g, _ := gardenAt(t, map[string]map[string]int{"1970-01-01": {"A": 600}, "2026-10-20": {"A": 600}})
	if n := strings.Count(plain(strings.Join(g.body(gardenDays(g.books)), "\n")), "Mon "); n > 60 {
		t.Errorf("garden drew %d weeks for a 1970 day", n)
	}
}

func TestGardenNarrow(t *testing.T) {
	g, _ := gardenAt(t, map[string]map[string]int{"2026-10-20": {"A": 600}})
	for _, line := range strings.Split(g.view(50, 40), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line %d wide on a 50-column screen: %q", w, plain(line))
		}
	}
}
