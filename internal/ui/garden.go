package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/garden"
)

// The garden: one tree per day sized by reading time, a row per week
// (Monday first), months as headings, the legend and goal at the bottom.

const (
	treeW   = 9
	weekW   = 7*treeW + 6 // a blank column between trees
	weekH   = 8           // 5 tree rows, day, minutes, blank
	legendH = 8           // rule, 5 tree rows, labels, goal
)

type gardenScreen struct {
	books   map[string]map[string]int // day -> book title -> seconds
	goal    int
	setGoal func(int)
	today   time.Time // midnight of today's reading day
	sel     time.Time
	top     int  // first body line shown; -1 = scrolled to the end
	follow  bool // keep the selected week on screen
	cells   []gardenCell
	left    int // screen column of the garden, for clicks
	bodyY   int // screen row of the first body line
	bodyH   int // body lines shown
}

type gardenCell struct {
	x, y int // relative to the body
	day  time.Time
}

func newGarden(books map[string]map[string]int, goal int, setGoal func(int), now time.Time) *gardenScreen {
	today := garden.DayTime(now)
	return &gardenScreen{books: books, goal: goal, setGoal: setGoal, today: today, sel: today, top: -1}
}

func gardenDays(books map[string]map[string]int) garden.Days {
	d := garden.Days{}
	for day, bs := range books {
		for _, s := range bs {
			d[day] += s
		}
	}
	return d
}

func center(s string, w int) string {
	pad := max(0, w-runewidth.StringWidth(s))
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

// weekRow draws a week: trees, then the day and its minutes under each.
// Days after today show a faint dot; the selected day's label is reversed.
func weekRow(days garden.Days, monday, today, sel time.Time, goal int) []string {
	trees := make([][]string, 7)
	labels, mins := make([]string, 7), make([]string, 7)
	for i := range 7 {
		d := monday.AddDate(0, 0, i)
		secs := days[garden.Key(d)]
		label := center(d.Format("Mon 2"), treeW)
		switch {
		case d.After(today):
			trees[i] = []string{"", "", "", "", faint.Render(center("·", treeW))}
			labels[i], mins[i] = faint.Render(label), strings.Repeat(" ", treeW)
		default:
			trees[i] = garden.Tree(garden.Level(secs, goal), !noColor)
			labels[i] = faint.Render(label)
			mins[i] = strings.Repeat(" ", treeW)
			if secs >= 60 {
				mins[i] = center(garden.FormatMinutes(secs), treeW)
			}
		}
		if d.Equal(sel) {
			labels[i] = reverse.Render(label)
		}
		for r := range trees[i] {
			if trees[i][r] == "" {
				trees[i][r] = strings.Repeat(" ", treeW)
			}
		}
	}
	out := make([]string, 0, 7)
	for r := range 5 {
		row := make([]string, 7)
		for i := range 7 {
			row[i] = trees[i][r]
		}
		out = append(out, strings.Join(row, " "))
	}
	return append(out, strings.Join(labels, " "), strings.Join(mins, " "))
}

// gardenSummary is "♣ 5 days in a row · today 12m of 30m".
func gardenSummary(days garden.Days, today time.Time, goal int) string {
	s := fmt.Sprintf("today %s of %dm", garden.FormatMinutes(days[garden.Key(today)]), goal)
	if n := garden.Streak(days, today); n > 0 {
		s = fmt.Sprintf("%d days in a row · ", n) + s
	}
	return "♣ " + s
}

// body is every week from the first day with reading to this week, with a
// heading wherever a week's Sunday starts a new month.
func (g *gardenScreen) body(days garden.Days) []string {
	first := garden.Monday(g.today)
	for k := range days {
		// a day before 2000 is a clock that was wrong, not a reader
		if d, ok := garden.ParseDay(k); ok && d.Before(first) && k >= "2000-01-01" {
			first = garden.Monday(d)
		}
	}
	var lines []string
	g.cells = g.cells[:0]
	var month time.Month
	for w := first; !w.After(g.today); w = w.AddDate(0, 0, 7) {
		if sun := w.AddDate(0, 0, 6); sun.Month() != month {
			month = sun.Month()
			read, longest := garden.MonthStats(days, sun.Year(), month, g.today)
			name := month.String()
			if sun.Year() != g.today.Year() {
				name += fmt.Sprint(" ", sun.Year())
			}
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, bold.Render(name)+faint.Render(fmt.Sprintf(" · %d days read · longest streak %d", read, longest)), "")
		}
		for i := range 7 {
			g.cells = append(g.cells, gardenCell{x: i * (treeW + 1), y: len(lines), day: w.AddDate(0, 0, i)})
		}
		lines = append(lines, weekRow(days, w, g.today, g.sel, g.goal)...)
		lines = append(lines, "")
	}
	if len(days) == 0 {
		lines = append(lines, faint.Render("read a book and your first tree grows here"))
	}
	return lines
}

// legend draws each level as a tree with its minutes for the goal.
func (g *gardenScreen) legend() []string {
	out := []string{faint.Render(strings.Repeat("─", weekW))}
	labels := garden.Labels(g.goal)
	trees := make([][]string, garden.MaxLevel)
	for i := range trees {
		trees[i] = garden.Tree(i+1, !noColor)
	}
	for r := range 5 {
		row := make([]string, garden.MaxLevel)
		for i := range row {
			row[i] = trees[i][r]
		}
		out = append(out, strings.Join(row, " "))
	}
	row := make([]string, garden.MaxLevel)
	for i, l := range labels {
		row[i] = faint.Render(center(l, treeW))
	}
	return append(out, strings.Join(row, " "), faint.Render(fmt.Sprintf("goal %dm/day · + −", g.goal)))
}

// selStatus describes the selected day: its time and the books read.
func (g *gardenScreen) selStatus() string {
	day := g.sel.Format("Mon 2")
	books := g.books[garden.Key(g.sel)]
	total := 0
	type bt struct {
		title string
		secs  int
	}
	var list []bt
	for t, s := range books {
		total += s
		if s >= 60 {
			list = append(list, bt{t, s})
		}
	}
	if total < 60 {
		if g.sel.Equal(g.today) {
			return day + " · nothing read yet"
		}
		return day + " · nothing read"
	}
	slices.SortFunc(list, func(a, b bt) int { return cmp.Or(cmp.Compare(b.secs, a.secs), strings.Compare(a.title, b.title)) })
	parts := make([]string, len(list))
	for i, b := range list {
		parts[i] = b.title + " " + garden.FormatMinutes(b.secs)
	}
	return day + " · " + garden.FormatMinutes(total) + " · " + strings.Join(parts, ", ")
}

func (g *gardenScreen) view(w, h int) string {
	days := gardenDays(g.books)
	width := min(w, weekW)
	g.left = max(0, (w-width)/2)
	margin := strings.Repeat(" ", g.left)
	head := []string{bold.Render("garden") + faint.Render("  "+gardenSummary(days, g.today, g.goal)), ""}
	foot := append(g.legend(), "", faint.Render(runewidth.Truncate(g.selStatus(), width, "…")))
	body := g.body(days)
	bodyH := max(1, h-len(head)-len(foot)-2)
	g.bodyH = bodyH
	maxTop := max(0, len(body)-bodyH)
	if g.top < 0 {
		g.top = maxTop
	}
	if g.follow {
		for _, c := range g.cells {
			if c.day.Equal(g.sel) {
				if c.y < g.top {
					g.top = c.y
				} else if c.y+weekH > g.top+bodyH {
					g.top = c.y + weekH - bodyH
				}
			}
		}
	}
	g.top = max(0, min(g.top, maxTop))
	var rows []string
	for _, l := range head {
		rows = append(rows, margin+l)
	}
	for i := g.top; i < min(len(body), g.top+bodyH); i++ {
		rows = append(rows, margin+body[i])
	}
	for _, l := range foot {
		rows = append(rows, margin+l)
	}
	// centered on the screen as one block, like the reading frame
	pad := max(0, (h-len(rows))/2)
	g.bodyY = pad + len(head)
	rows = append(make([]string, pad), rows...)
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, w, "") // narrow screens cut the trees cleanly
	}
	return strings.Join(rows[:min(len(rows), h)], "\n")
}

// key handles a key; it reports whether the garden should close.
func (g *gardenScreen) key(k string) bool {
	move := func(days int) {
		d := g.sel.AddDate(0, 0, days)
		if !d.After(g.today) {
			g.sel = d
		}
		g.follow = true
	}
	switch k {
	case "esc", "q", "ctrl+g":
		return true
	case "left", "h":
		move(-1)
	case "right", "l":
		move(1)
	case "up":
		move(-7)
	case "down":
		move(7)
	case "k":
		g.top, g.follow = max(0, g.top-weekH), false
	case "j":
		g.top, g.follow = g.top+weekH, false
	case "+", "=":
		g.changeGoal(5)
	case "-":
		g.changeGoal(-5)
	}
	return false
}

func (g *gardenScreen) changeGoal(d int) {
	g.goal = max(5, min(120, g.goal+d))
	if g.setGoal != nil {
		g.setGoal(g.goal)
	}
}

// mouse scrolls with the wheel and selects the day under a click.
func (g *gardenScreen) mouse(msg tea.MouseMsg) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		g.key("k")
	case msg.Button == tea.MouseButtonWheelDown:
		g.key("j")
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		x, y := msg.X-g.left, msg.Y-g.bodyY+g.top
		if msg.Y < g.bodyY || msg.Y >= g.bodyY+g.bodyH {
			return // the legend, the heading, or weeks scrolled away
		}
		for _, c := range g.cells {
			if x >= c.x && x < c.x+treeW && y >= c.y && y < c.y+weekH-1 && !c.day.After(g.today) {
				g.sel = c.day
			}
		}
	}
}
