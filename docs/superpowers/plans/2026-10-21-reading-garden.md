# Reading Garden Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Count the minutes spent reading in tome every day, across all books, and show them as a pixel-art garden: one tree per day, a row per week, grouped by month.

**Architecture:**
- A new pure package `internal/garden` holds the logic: days, the idle counter, tree levels and pixel grids, and streaks. It has no UI and no file access.
- `internal/state` stores the seconds per day in each book's state file, merging what the reader hands over on every save.
- `internal/ui` feeds activity into the counter while a book is open, shows today's time in the status line, and draws the garden screen, which both the reader and the picker open.

**Tech Stack:** Go 1.26, Bubble Tea v1.3.10 (`tea.WithReportFocus`, `tea.FocusMsg`, `tea.BlurMsg`), lipgloss, `github.com/charmbracelet/x/ansi` for widths.

**Spec:** `docs/superpowers/specs/2026-10-21-reading-garden-design.md` (approved preview: `docs/superpowers/specs/2026-10-21-reading-garden.png`)

## Global Constraints

- Idle cap: a gap between two activities adds at most **3 minutes**.
- A reading day runs **04:00 to 04:00** local time. A gap across 04:00 is split between the two days.
- Default goal **30** minutes a day. `+`/`-` change it in steps of **5**, within **5 to 120**.
- Levels against the goal: 0 (none) bare ground; under 50% sprout; 50–100% sapling; 100–150% tree; 150–200% big tree; 200–400% flowers; 400% or more flowers and fruit.
- Pixel colors are ANSI palette indexes: leaf 2, light leaf 10, trunk 3, flower 13, fruit 9, ground 8. With `NO_COLOR`, draw blocks with no color.
- Trees are 9 columns × 5 rows; a week row is 69 columns (one blank column between trees).
- Streak: days in a row with at least 60 seconds. If today has less, the streak counts up to yesterday.
- The `ctrl+g` key opens the garden in both the picker and the reader. The reader menu gets a "garden" entry with that key.
- Repo rules:
  - Match the surrounding code style.
  - Run `gofmt` on every changed file.
  - README text has no em or en dashes; ranges inside the UI may use `–`.
- **No commits during the plan.** Each task ends with `go test ./...` passing. The user tests the build and approves before the single commit and release.

## Review Focus

- **Terminal that never reports focus:** time must still count from the moment the book opens, without any `FocusMsg`. Tested in Task 4.
- **Clock going backwards** (manual change, NTP step): a negative gap must add nothing and must not break later counting. Tested in Task 1.
- **Two tome windows, even on the same book:** both windows' minutes must survive. Each save adds a delta to what the file holds right then. Tested in Task 3.
- **Old state and prefs files without the new fields:** they load fine, with goal 30 and no reading data. Tested in Task 3.
- **Short terminal in the picker:** the week strip shrinks to the summary line or disappears, and clicks on book rows still open the right book. Tested in Task 6.

---

### Task 1: Days and the idle counter (`internal/garden`)

**Files:**
- Create: `internal/garden/days.go`
- Create: `internal/garden/counter.go`
- Test: `internal/garden/garden_test.go`

**Interfaces:**
- Produces:
  - `const DayStart = 4 * time.Hour`
  - `const Idle = 3 * time.Minute`
  - `func DayTime(t time.Time) time.Time`: midnight (local) of t's reading day.
  - `func Key(day time.Time) string`: `"2006-01-02"`.
  - `func DayOf(t time.Time) string` = `Key(DayTime(t))`.
  - `func ParseDay(s string) (time.Time, bool)`: midnight local.
  - `func Monday(day time.Time) time.Time`.
  - `type Credit struct { Day string; D time.Duration }`
  - `type Counter struct` with `Start(now time.Time)`, `Activity(now time.Time) []Credit` and `Pause(now time.Time) []Credit`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/garden/`
Expected: FAIL to build (`undefined: DayOf`, `Counter`…).

- [ ] **Step 3: Implement `days.go`**

```go
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
```

- [ ] **Step 4: Implement `counter.go`**

```go
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
func (c *Counter) Start(now time.Time) { c.last, c.active = now, true }

// Activity records an event and returns the time since the previous one,
// capped at Idle and split at day boundaries. While paused it only starts.
func (c *Counter) Activity(now time.Time) []Credit {
	if !c.active {
		c.Start(now)
		return nil
	}
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/garden/`
Expected: PASS

---

### Task 2: Levels, labels, trees and streaks (`internal/garden`)

**Files:**
- Create: `internal/garden/tree.go`
- Create: `internal/garden/stats.go`
- Test: `internal/garden/tree_test.go`

**Interfaces:**
- Consumes: `Key`, `ParseDay`, `DayTime` from Task 1.
- Produces:
  - `const MaxLevel = 6`
  - `func Level(secs, goalMin int) int`: 0 to 6.
  - `func Labels(goalMin int) [MaxLevel]string`: legend labels for levels 1 to 6.
  - `func FormatMinutes(secs int) string`: `"0m"`, `"34m"`, `"1h"`, `"1h20"`.
  - `func Tree(level int, color bool) []string`: 5 lines, each 9 cells wide.
  - `type Days map[string]int`: day key to seconds, all books.
  - `func Streak(d Days, today time.Time) int`
  - `func MonthStats(d Days, year int, month time.Month, today time.Time) (daysRead, longest int)`

- [ ] **Step 1: Write the failing tests**

```go
package garden

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLevels(t *testing.T) {
	for secs, want := range map[int]int{
		0: 0, 30: 1, 14 * 60: 1, 15 * 60: 2, 30 * 60: 3, 45 * 60: 4,
		60 * 60: 5, 119 * 60: 5, 120 * 60: 6, 300 * 60: 6,
	} {
		if got := Level(secs, 30); got != want {
			t.Errorf("Level(%dm) = %d, want %d", secs/60, got, want)
		}
	}
}

func TestLabelsFollowGoal(t *testing.T) {
	want := [MaxLevel]string{"<15m", "15–30m", "30–45m", "45m–1h", "1–2h", "2h+"}
	if got := Labels(30); got != want {
		t.Errorf("Labels(30) = %q", got)
	}
	if got := Labels(60); got[0] != "<30m" || got[5] != "4h+" {
		t.Errorf("Labels(60) = %q", got)
	}
	for secs, want := range map[int]string{0: "0m", 59: "0m", 34 * 60: "34m", 3600: "1h", 80 * 60: "1h20"} {
		if got := FormatMinutes(secs); got != want {
			t.Errorf("FormatMinutes(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestTreesAreNineByFive(t *testing.T) {
	for lvl := 0; lvl <= MaxLevel; lvl++ {
		for _, color := range []bool{true, false} {
			rows := Tree(lvl, color)
			if len(rows) != 5 {
				t.Fatalf("level %d: %d rows", lvl, len(rows))
			}
			for _, r := range rows {
				if w := ansi.StringWidth(r); w != 9 {
					t.Errorf("level %d color %v: width %d in %q", lvl, color, w, r)
				}
			}
		}
	}
	if strings.Contains(strings.Join(Tree(4, false), ""), "\x1b") {
		t.Error("NO_COLOR tree has escape codes")
	}
	if !strings.Contains(strings.Join(Tree(6, true), ""), "\x1b[91") { // fruit: bright red
		t.Error("fruit tree has no fruit color")
	}
}

func TestStreak(t *testing.T) {
	today := at("2026-10-21 12:00:00")
	d := Days{"2026-10-18": 600, "2026-10-19": 600, "2026-10-20": 600}
	if got := Streak(d, DayTime(today)); got != 3 { // today not read yet: keep yesterday's run
		t.Errorf("streak before reading today = %d", got)
	}
	d["2026-10-21"] = 120
	if got := Streak(d, DayTime(today)); got != 4 {
		t.Errorf("streak = %d", got)
	}
	d["2026-10-19"] = 30 // under a minute breaks it
	if got := Streak(d, DayTime(today)); got != 2 {
		t.Errorf("streak with a short day = %d", got)
	}
}

func TestMonthStats(t *testing.T) {
	d := Days{"2026-10-01": 600, "2026-10-02": 600, "2026-10-05": 600, "2026-09-30": 600}
	read, longest := MonthStats(d, 2026, 10, DayTime(at("2026-10-21 12:00:00")))
	if read != 3 || longest != 2 {
		t.Errorf("read %d longest %d", read, longest)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/garden/`
Expected: FAIL to build (`undefined: Level`…).

- [ ] **Step 3: Implement `tree.go`**

```go
package garden

import (
	"fmt"
	"strings"
)

// MaxLevel is the biggest tree: flowers and fruit.
const MaxLevel = 6

// Level sizes a day's tree from its reading time against the daily goal:
// 0 bare ground, 1 sprout, 2 sapling, 3 tree (goal met), 4 big tree,
// 5 flowers (twice the goal), 6 flowers and fruit (four times).
func Level(secs, goalMin int) int {
	if secs <= 0 {
		return 0
	}
	r := float64(secs) / float64(goalMin*60)
	switch {
	case r < 0.5:
		return 1
	case r < 1:
		return 2
	case r < 1.5:
		return 3
	case r < 2:
		return 4
	case r < 4:
		return 5
	}
	return MaxLevel
}

// Labels are the legend's minutes for levels 1 to MaxLevel.
func Labels(goalMin int) [MaxLevel]string {
	g := goalMin
	return [MaxLevel]string{
		"<" + minutes(g/2),
		span(g/2, g),
		span(g, g*3/2),
		span(g*3/2, g*2),
		span(g*2, g*4),
		minutes(g*4) + "+",
	}
}

// span is a range of minutes, written once when both ends share a unit.
func span(a, b int) string {
	switch {
	case b < 60:
		return fmt.Sprintf("%d–%dm", a, b)
	case a >= 60 && a%60 == 0 && b%60 == 0:
		return fmt.Sprintf("%d–%dh", a/60, b/60)
	}
	return minutes(a) + "–" + minutes(b)
}

func minutes(m int) string {
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", m)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

// FormatMinutes writes seconds as whole minutes: 34m, 1h, 1h20.
func FormatMinutes(secs int) string { return minutes(secs / 60) }

// Pixel art, 9 wide and 10 tall: . empty, L light leaf, g leaf, T trunk,
// F flower, R fruit, d ground. Two pixel rows make one terminal row.
var grids = [MaxLevel + 1][10]string{
	{".........", ".........", ".........", ".........", ".........", ".........", ".........", ".........", ".........", "...ddd..."},
	{".........", ".........", ".........", ".........", ".........", ".........", ".........", "..LL.gg..", "....g....", "....g...."},
	{".........", ".........", ".........", ".........", ".........", "....L....", "...LLg...", "...Lgg...", "....T....", "....T...."},
	{".........", ".........", "....L....", "...LLg...", "..LLggg..", "..Lgggg..", "...ggg...", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LLLLg..", ".LLLgggg.", "LLggggggg", "Lgggggggg", ".ggggggg.", "..ggggg..", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LFLLg..", ".LLLgFgg.", "LLgggggFg", "LgFgggggg", ".ggggFgg.", "..ggggg..", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LFLLg..", ".LLLgFgg.", "LLggRggFg", "LgFgggRgg", ".gRggFgg.", "..ggRgg..", "....T....", "....T....", "...TTT..."},
}

// palette maps pixels to ANSI colors, so the terminal's theme picks shades.
var palette = map[byte]int{'g': 2, 'L': 10, 'T': 3, 'F': 13, 'R': 9, 'd': 8}

// Tree draws a level as 5 rows of 9 cells with half blocks: the top pixel
// is the foreground of ▀, the bottom one its background.
func Tree(level int, color bool) []string {
	g := grids[max(0, min(level, MaxLevel))]
	rows := make([]string, 0, 5)
	for r := 0; r < 10; r += 2 {
		var sb strings.Builder
		for c := 0; c < 9; c++ {
			sb.WriteString(cell(g[r][c], g[r+1][c], color))
		}
		rows = append(rows, sb.String())
	}
	return rows
}

func cell(top, bot byte, color bool) string {
	switch {
	case top == '.' && bot == '.':
		return " "
	case !color && top == '.':
		return "▄"
	case !color && bot == '.':
		return "▀"
	case !color:
		return "█"
	case top == bot:
		return sgr(fg(palette[top])) + "█\x1b[0m"
	case top == '.':
		return sgr(fg(palette[bot])) + "▄\x1b[0m"
	case bot == '.':
		return sgr(fg(palette[top])) + "▀\x1b[0m"
	}
	return sgr(fg(palette[top]), bg(palette[bot])) + "▀\x1b[0m"
}

func fg(n int) int {
	if n < 8 {
		return 30 + n
	}
	return 90 + n - 8
}

func bg(n int) int { return fg(n) + 10 }

func sgr(codes ...int) string {
	s := make([]string, len(codes))
	for i, c := range codes {
		s[i] = fmt.Sprint(c)
	}
	return "\x1b[" + strings.Join(s, ";") + "m"
}
```

- [ ] **Step 4: Implement `stats.go`**

```go
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/garden/`
Expected: PASS

---

### Task 3: Store reading time (`internal/state`)

**Files:**
- Modify: `internal/state/state.go` (Position, Prefs, new functions)
- Test: `internal/state/state_test.go`

**Interfaces:**
- Produces:
  - `Position.Reading map[string]int` (`json:"reading,omitempty"`): day key to seconds, as stored.
  - `Position.ReadingAdd map[string]int` (`json:"-"`): seconds counted since the last save, handed from the reader to the save callback.
  - `func MergeReading(saved, add map[string]int) map[string]int`
  - `Prefs.DailyMinutes int` (`json:"daily_minutes,omitempty"`) and `func (p Prefs) Goal() int` (30 when unset).
  - `func Reading(dir, skip string) map[string]map[string]int`: day to book title to seconds, over every state file except key `skip`.

- [ ] **Step 1: Write the failing tests** (append to `state_test.go`)

```go
func TestReadingMergesDeltas(t *testing.T) {
	dir := t.TempDir()
	// two windows on the same book each hand over their own delta
	for _, add := range []map[string]int{{"2026-10-21": 300}, {"2026-10-21": 120, "2026-10-22": 60}} {
		p := Load(dir, "k")
		p.Reading = MergeReading(p.Reading, add)
		if err := Save(dir, "k", p); err != nil {
			t.Fatal(err)
		}
	}
	got := Load(dir, "k").Reading
	if got["2026-10-21"] != 420 || got["2026-10-22"] != 60 {
		t.Errorf("reading = %v", got)
	}
}

func TestReadingAcrossBooks(t *testing.T) {
	dir := t.TempDir()
	Save(dir, "a", Position{Title: "A", Path: "/gone/a.epub", Reading: map[string]int{"2026-10-21": 600}})
	Save(dir, "b", Position{Title: "B", Reading: map[string]int{"2026-10-21": 60, "2026-10-20": 30}})
	SavePrefs(dir, Prefs{})
	all := Reading(dir, "")
	if all["2026-10-21"]["A"] != 600 || all["2026-10-21"]["B"] != 60 || all["2026-10-20"]["B"] != 30 {
		t.Errorf("all = %v", all) // a book whose file is gone still counts
	}
	if got := Reading(dir, "a"); got["2026-10-21"]["A"] != 0 {
		t.Errorf("skip kept a: %v", got)
	}
}

func TestOldFilesLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"chapter":2,"block":5}`), 0o644)
	os.WriteFile(filepath.Join(dir, "prefs.json"), []byte(`{"hide_toc":true}`), 0o644)
	if p := Load(dir, "old"); p.Chapter != 2 || p.Reading != nil {
		t.Errorf("old position = %+v", p)
	}
	if g := LoadPrefs(dir).Goal(); g != 30 {
		t.Errorf("default goal = %d", g)
	}
}
```

Make sure `os` and `path/filepath` are imported in the test file.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/state/`
Expected: FAIL to build (`unknown field Reading`, `undefined: MergeReading`…).

- [ ] **Step 3: Implement**

In `Position`, after `Bookmarks`:

```go
	// Reading is seconds read per day ("2006-01-02"), for the garden.
	Reading map[string]int `json:"reading,omitempty"`
	// ReadingAdd is what the reader counted since its last save. It is
	// added to the file's Reading at save time, never written itself, so
	// two windows can't overwrite each other's time.
	ReadingAdd map[string]int `json:"-"`
```

In `Prefs`:

```go
	DailyMinutes int  `json:"daily_minutes,omitempty"` // the garden's goal; 0 = default
```

New functions after `Recent`:

```go
// MergeReading adds seconds per day to saved ones.
func MergeReading(saved, add map[string]int) map[string]int {
	if len(add) == 0 {
		return saved
	}
	if saved == nil {
		saved = map[string]int{}
	}
	for day, s := range add {
		saved[day] += s
	}
	return saved
}

// Reading collects seconds per day per book title from every state file
// except skip, including books whose file has since gone.
func Reading(dir, skip string) map[string]map[string]int {
	out := map[string]map[string]int{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.Name() == "prefs.json" || key == skip {
			continue
		}
		p := Load(dir, key)
		title := p.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path))
		}
		for day, s := range p.Reading {
			if out[day] == nil {
				out[day] = map[string]int{}
			}
			out[day][title] += s
		}
	}
	return out
}
```

After `LoadPrefs`:

```go
// Goal is the daily reading goal in minutes.
func (p Prefs) Goal() int {
	if p.DailyMinutes <= 0 {
		return 30
	}
	return p.DailyMinutes
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/state/`
Expected: PASS

---

### Task 4: Count reading time in the reader

**Files:**
- Create: `internal/ui/reading.go`
- Modify: `internal/ui/ui.go`:
  - `Model` struct (line 54);
  - `New` (line 210);
  - `Init` (line 226);
  - `persist` (line 358);
  - `Update` (line 413);
  - `update` (add a `readTickMsg` case);
  - `status` (line 977).
- Modify: `cmd/tome/main.go`:
  - the save callback (lines 147–156);
  - `tea.NewProgram` for the reader (line 191).
- Test: `internal/ui/reading_test.go`

**Interfaces:**
- Consumes:
  - `garden.Counter`, `garden.Credit`, `garden.DayOf`, `garden.FormatMinutes` (Tasks 1 and 2);
  - `state.Position.ReadingAdd`, `state.Position.Reading`, `state.MergeReading`, `state.Reading`, `state.Prefs.Goal` (Task 3).
- Produces:
  - `func (m *Model) SetClock(now func() time.Time)`: tests only.
  - `func (m *Model) SetGarden(others map[string]map[string]int, goal int, setGoal func(int))`
  - `func (m *Model) gardenBooks() map[string]map[string]int`: every book including this one's live time. Used by Task 5.
  - Fields `m.goal int` and `m.setGoal func(int)`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'Reading|StatusShowsToday'`
Expected: FAIL to build (`m.SetClock undefined`…).

- [ ] **Step 3: Create `internal/ui/reading.go`**

```go
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
```

- [ ] **Step 4: Wire it into `ui.go`**

Fields in `Model`, after `save func(state.Position)`:

```go
	// reading garden (reading.go)
	clock   func() time.Time
	counter garden.Counter
	mine    map[string]time.Duration // this book's time per day, saved and new
	unsaved map[string]time.Duration // counted but not yet handed to save
	others  map[string]map[string]int
	goal    int // minutes a day; 0 until SetGarden
	setGoal func(int)
```

In `New`, where the model is built:

```go
	m.clock = time.Now
	m.mine, m.unsaved = map[string]time.Duration{}, map[string]time.Duration{}
	for day, s := range pos.Reading {
		m.mine[day] = time.Duration(s) * time.Second
	}
	m.counter.Start(m.clock()) // opening the book counts as activity
```

`Init`:

```go
func (m *Model) Init() tea.Cmd { return tea.Batch(m.initCmd, readTick()) }
```

`persist`:

```go
func (m *Model) persist() {
	if m.save != nil && m.doc != nil {
		p := m.position()
		p.ReadingAdd = m.drainReading()
		m.save(p)
	}
}
```

`Update`: call `m.track(msg)` as the first statement.

In `update`, a new case next to `caretIdleMsg`:

```go
	case readTickMsg:
		if len(m.unsaved) > 0 {
			m.persist()
		}
		return m, readTick()
```

`status`: replace the `pct :=` line with:

```go
	pct := fmt.Sprintf(" · %d%%", m.percent())
	if m.goal > 0 {
		pct += fmt.Sprintf(" · today %s/%dm", garden.FormatMinutes(m.todaySecs()), m.goal)
	}
```

Add the `garden` import. Check that the `ctrl+c` path calls `persist()` (it does today at `ui.go:508` and `ui.go:552`), so quitting hands over the last seconds.

- [ ] **Step 5: Wire `cmd/tome/main.go`**

Replace the save callback body, so prefs saved by the reader keep `DailyMinutes`, and reading time is merged with the file:

```go
	m := ui.New(book, pos, func(p state.Position) {
		pr := state.LoadPrefs(a.stateDir)
		pr.HideTOC, pr.TOCWidth, pr.ReadHeight, pr.Hinted = p.HideTOC, p.TOCWidth, p.ReadHeight, true
		if err := state.SavePrefs(a.stateDir, pr); err != nil {
			saveErr = err
		}
		p.HideTOC, p.TOCWidth, p.ReadHeight = false, 0, 0
		p.Path, p.Title, p.Author, p.Opened = abs, book.Title, book.Author, opened
		// add the new time to what the file holds now: another window may
		// have saved its own meanwhile
		p.Reading = state.MergeReading(state.Load(a.stateDir, key).Reading, p.ReadingAdd)
		if err := state.Save(a.stateDir, key, p); err != nil {
			saveErr = err
		}
	})
```

After `m.SetBookList(fromList)`:

```go
	m.SetGarden(state.Reading(a.stateDir, key), prefs.Goal(), a.setGoal)
```

Add a method on `app`:

```go
// setGoal saves the garden's daily goal in minutes.
func (a *app) setGoal(minutes int) {
	pr := state.LoadPrefs(a.stateDir)
	pr.DailyMinutes = minutes
	state.SavePrefs(a.stateDir, pr)
}
```

Reader program: `tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())`.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS. Existing tests build `ui.New` without `SetGarden`; `goal` stays 0, so their status lines are unchanged.

---

### Task 5: The garden screen, opened from the reader

**Files:**
- Create: `internal/ui/garden.go`
- Modify: `internal/ui/overlay.go`:
  - add `overlayGarden` to the `overlayKind` constants;
  - add `menuGarden` to the `menuAction` constants;
  - add the menu entry in `menuEntries`;
  - add `case overlayGarden` in `overlayKey`;
  - add `case menuGarden` in `activateMenu`.
- Modify: `internal/ui/ui.go`:
  - `key()`: `ctrl+g` opens the garden;
  - `View()`: draw the garden when it's open;
  - `case tea.MouseMsg`: route the mouse to the garden;
  - help list in `overlay.go` (line 97): add `ctrl+g`.
- Test: `internal/ui/garden_test.go`

**Interfaces:**
- Consumes:
  - `garden.Tree`, `garden.Level`, `garden.Labels`, `garden.FormatMinutes`, `garden.Streak`, `garden.MonthStats`, `garden.Monday`, `garden.DayTime`, `garden.Key`, `garden.ParseDay`, `garden.Days` (Tasks 1 and 2);
  - `m.gardenBooks()`, `m.goal`, `m.setGoal`, `m.clock` (Task 4).
- Produces:
  - `type gardenScreen struct` with:
    - `newGarden(books map[string]map[string]int, goal int, setGoal func(int), now time.Time) *gardenScreen`
    - `(g *gardenScreen) view(w, h int) string`
    - `(g *gardenScreen) key(k string) (closed bool)`
    - `(g *gardenScreen) mouse(msg tea.MouseMsg)`
  - `func weekRow(days garden.Days, monday, today, sel time.Time, goal int) []string`: 7 lines, used by the picker strip in Task 6.
  - `func gardenSummary(days garden.Days, today time.Time, goal int) string`

- [ ] **Step 1: Write the failing tests**

```go
package ui

import (
	"strings"
	"testing"
	"time"

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
```

Add `tea "github.com/charmbracelet/bubbletea"` to the imports of this test file.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run Garden`
Expected: FAIL to build (`undefined: newGarden`…).

- [ ] **Step 3: Create `internal/ui/garden.go`**

```go
package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/garden"
)

// The garden: one tree per day sized by reading time, a row per week
// (Monday first), months as headings, the legend and goal at the bottom.

const (
	treeW   = 9
	weekW   = 7*treeW + 6 // a blank column between trees
	weekH   = 8           // 5 tree rows, day, minutes, blank
	legendH = 7           // rule, 5 tree rows, labels
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
		if d, ok := garden.ParseDay(k); ok && d.Before(first) {
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
	return append(out, strings.Join(row, " ")+faint.Render(fmt.Sprintf("  goal %dm/day · + −", g.goal)))
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
	g.bodyY = 1 + len(head)
	rows := []string{""}
	for _, l := range head {
		rows = append(rows, margin+l)
	}
	for i := g.top; i < min(len(body), g.top+bodyH); i++ {
		rows = append(rows, margin+body[i])
	}
	for len(rows) < h-len(foot) {
		rows = append(rows, "")
	}
	for _, l := range foot {
		rows = append(rows, margin+l)
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
		for _, c := range g.cells {
			if x >= c.x && x < c.x+treeW && y >= c.y && y < c.y+weekH-1 && !c.day.After(g.today) {
				g.sel = c.day
			}
		}
	}
}
```

- [ ] **Step 4: Open it from the reader**

`overlay.go`:
- Add `overlayGarden` after `overlayBookmarks` in the `overlayKind` constants.
- Add `menuGarden` before `menuWeb` in `menuAction`.
- In `menuEntries`, after the bookmarks entry: `{"garden", "ctrl+g", menuGarden},`.
- In the help list (line ~97), after the bookmarks line: `{"ctrl+g", "reading garden: a tree for each day you read"},`.
- In `overlayKey`:

```go
	case overlayGarden:
		if m.garden.key(k) {
			m.overlay, m.garden = overlayNone, nil
		}
```

- In `activateMenu`:

```go
	case menuGarden:
		m.openGarden()
```

`reading.go`, add:

```go
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
```

`ui.go`:
- Add the field `garden *gardenScreen` to `Model`.
- In `key(k)`, with the other global keys before the `switch` that handles reading keys: `case "ctrl+g": m.openGarden(); return nil`.
- At the start of `View()`, after the `m.doc == nil` check:

```go
	if m.overlay == overlayGarden && m.garden != nil {
		return m.garden.view(m.w, m.h)
	}
```

- At the top of `case tea.MouseMsg:` in `update`:

```go
		if m.overlay == overlayGarden && m.garden != nil {
			m.garden.mouse(msg)
			return m, nil
		}
```

Check how `key()` reaches overlays: if overlay keys are dispatched before the `ctrl+g` case, `ctrl+g` inside the garden goes to `gardenScreen.key`, which closes it (handled above).

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ui/`
Expected: PASS. If `TestGardenScreen` fails only on the exact minute label for Tue 20 (52m + 20m = 72m = "1h12"), check `garden.FormatMinutes`.

---

### Task 6: The week strip and `ctrl+g` in the picker

**Files:**
- Modify: `internal/ui/picker.go`:
  - `Picker` struct;
  - `listH` (line 167);
  - `Update` (line 169);
  - `View` (line 243).
- Modify: `cmd/tome/main.go`: the picker loop (line 110).
- Test: `internal/ui/picker_test.go`

**Interfaces:**
- Consumes: `newGarden`, `weekRow`, `gardenSummary`, `gardenDays` (Task 5); `garden.Monday`, `garden.DayTime` (Task 1).
- Produces: `func (p *Picker) SetGarden(books map[string]map[string]int, goal int, setGoal func(int))`; field `p.now func() time.Time` (tests set it).

- [ ] **Step 1: Write the failing tests** (append to `picker_test.go`)

```go
func gardenPicker(t *testing.T, h int) *Picker {
	t.Helper()
	lib, root := scanned(t, "a.epub", "b.epub")
	p := NewPicker(lib, nil, root, "", "", 0)
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-21 20:00", time.Local)
	p.now = func() time.Time { return now }
	p.SetGarden(map[string]map[string]int{"2026-10-20": {"A": 40 * 60}}, 30, nil)
	p.Update(tea.WindowSizeMsg{Width: 100, Height: h})
	return p
}

func TestPickerShowsWeek(t *testing.T) {
	p := gardenPicker(t, 40)
	out := ansi.Strip(p.View())
	if !strings.Contains(out, "♣ 1 days in a row · today 0m of 30m") || !strings.Contains(out, "Tue 20") {
		t.Fatalf("no week strip:\n%s", out)
	}
	// clicks on a book row still open that book
	for y, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "▸ a") { // the cursor starts on the first book
			p.Update(tea.MouseMsg{X: 10, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			break
		}
	}
	if !strings.HasSuffix(p.Chosen, "a.epub") {
		t.Errorf("clicked row opened %q", p.Chosen)
	}
}

func TestPickerShortHidesTrees(t *testing.T) {
	p := gardenPicker(t, 14)
	out := ansi.Strip(p.View())
	if strings.Contains(out, "Tue 20") {
		t.Errorf("trees shown on a short screen:\n%s", out)
	}
}

func TestPickerOpensGarden(t *testing.T) {
	p := gardenPicker(t, 40)
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	if out := ansi.Strip(p.View()); !strings.Contains(out, "goal 30m/day") {
		t.Fatalf("ctrl+g didn't open the garden:\n%s", out)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if out := ansi.Strip(p.View()); !strings.Contains(out, "pick a book") {
		t.Errorf("esc didn't return to the list:\n%s", out)
	}
}
```

Add `github.com/charmbracelet/x/ansi` to the test imports. Book titles are file names without `.epub`, so the list holds `a` and `b`, with the cursor on `a`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run Picker`
Expected: FAIL to build (`p.SetGarden undefined`).

- [ ] **Step 3: Implement**

Fields in `Picker`:

```go
	// reading garden: this week's trees above the list, ctrl+g for all
	books   map[string]map[string]int
	goal    int
	setGoal func(int)
	now     func() time.Time
	garden  *gardenScreen
```

In `NewPicker`, set `now: time.Now` in the struct literal.

```go
// SetGarden shows this week's trees above the list.
func (p *Picker) SetGarden(books map[string]map[string]int, goal int, setGoal func(int)) {
	p.books, p.goal = books, goal
	p.setGoal = func(g int) {
		p.goal = g
		if setGoal != nil {
			setGoal(g)
		}
	}
}

// stripH is the rows the garden takes above the list: summary, the week
// and a blank line, only the summary when that would squeeze the list
// below six rows, or nothing.
func (p *Picker) stripH() int {
	if p.books == nil {
		return 0
	}
	free := p.frameH() - 5
	switch {
	case free-9 >= 6:
		return 9
	case free-2 >= 6:
		return 2
	}
	return 0
}
```

`listH`: `func (p *Picker) listH() int { return max(1, p.frameH()-5-p.stripH()) }`

At the top of `Update`, before the `switch`:

```go
	if p.garden != nil {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.Type == tea.KeyCtrlC {
				return p, tea.Quit
			}
			if p.garden.key(msg.String()) {
				p.garden = nil
			}
			return p, nil
		case tea.MouseMsg:
			p.garden.mouse(msg)
			return p, nil
		}
	}
```

In the key switch add `case tea.KeyCtrlG:` that calls `p.openGarden()` when `p.books != nil`.

In the mouse click case, use the strip height. A click on the strip opens the garden:

```go
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
			y := msg.Y - p.frameTop()
			top := 3 + p.stripH()
			if y >= 3 && y < top && p.books != nil {
				p.openGarden()
				return p, nil
			}
			if row := y - top + p.offset; y >= top && row < len(p.rowItem) && p.rowItem[row] >= 0 {
				p.cursor = p.rowItem[row]
				return p, p.open()
			}
```

```go
func (p *Picker) openGarden() {
	p.garden = newGarden(p.books, p.goal, p.setGoal, p.now())
}
```

`View`: first, after the `p.w == 0` check:

```go
	if p.garden != nil {
		return p.garden.view(p.w, p.h)
	}
```

After the three header rows (`tome`, the query, `""`):

```go
	if sh := p.stripH(); sh > 0 {
		days := gardenDays(p.books)
		today := garden.DayTime(p.now())
		rows = append(rows, margin+faint.Render(gardenSummary(days, today, p.goal)))
		if sh == 9 {
			for _, l := range weekRow(days, garden.Monday(today), today, time.Time{}, p.goal) {
				rows = append(rows, margin+l)
			}
		}
		rows = append(rows, "")
	}
```

Change the hint to `"↑↓ select · enter open · ctrl+g garden · esc quit"` when `p.books != nil`.

Add the `garden` import.

- [ ] **Step 4: Wire `main.go`**

In `run`, after `p := ui.NewPicker(...)`:

```go
		p.SetGarden(state.Reading(dir, ""), state.LoadPrefs(dir).Goal(), a.setGoal)
```

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS, including the existing picker tests, which don't call `SetGarden` and so draw no strip.

---

### Task 7: Docs, a real-terminal check, install

**Files:**
- Modify: `README.md` (feature list and keys)
- Modify: `docs/design.md` (new section)

- [ ] **Step 1: README**

In "Why tome", after "Search and bookmarks", add:

```markdown
- **Reading garden**: tome counts the minutes you spend reading each day, across all your books, and grows a tree for each day. Read longer and the tree grows bigger, then flowers and bears fruit. See this week above your books, or press `ctrl+g` for the whole garden.
```

In the key table, after the bookmarks row:

```markdown
| `ctrl+g` | your reading garden: a tree for each day you read |
```

In "Settings", extend the sentence about what is saved: `Your reading position, bookmarks, reading time and layout are saved…`.

Check: `grep -n "—\|–" README.md` prints nothing.

- [ ] **Step 2: design.md**

Add a section after "Browser view":

```markdown
## Reading garden (`internal/garden`, `ui/garden.go`, `ui/reading.go`)

Only time is measured, never pages or position, so reading forward, back or again all count. Any key, click, scroll or browser scroll in an open book is activity; the gap since the previous activity counts, capped at 3 minutes, so walking away or a sleeping laptop costs at most 3 minutes. Losing terminal focus stops the count (focus reporting), and opening a book starts it. A reading day runs from 04:00 to 04:00 and a gap across 04:00 is split. Each book's state file keeps seconds per day; the reader hands over only what it counted since its last save (every minute and on close), and the save adds it to what the file holds, so two windows never overwrite each other's time. The garden sums every state file.

Trees are 9×10 pixel art drawn with `▀` (top pixel as foreground, bottom as background), so a tree is 9 columns by 5 rows and a week fits the 72-column frame. Colors are palette indexes (leaves 2 and 10, trunk 3, flowers 13, fruit 9). The level is the day's time against the daily goal (30 minutes by default, `+`/`-` in the garden): under half a sprout, then sapling, tree at the goal, big tree at 1.5×, flowers at 2× and fruit at 4×. Trees are computed, never stored, so changing the goal redraws the past. The streak counts days with at least a minute and keeps yesterday's run until today is read.
```

- [ ] **Step 3: Full check and install**

Run:
```bash
gofmt -l . ; go vet ./... && go test ./... && GOBIN=$HOME/.local/bin go install -ldflags "-X main.version=dev" ./cmd/tome
```
Expected: no gofmt output, tests PASS, install succeeds.

- [ ] **Step 4: Check in a real terminal**

Run `tome`. The strip should show this week, with today as bare ground or a small tree. Open a book, read for a minute, and look for `today 1m/30m` in the status line. Press `ctrl+g` and compare the garden with `docs/superpowers/specs/2026-10-21-reading-garden.png`. Then hand over to the user to test; commit and release only after they approve.
