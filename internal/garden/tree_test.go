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

// Without color, flowers and fruit still change the shape.
func TestPlainTreesDiffer(t *testing.T) {
	big, flowers, fruit := strings.Join(Tree(4, false), "\n"), strings.Join(Tree(5, false), "\n"), strings.Join(Tree(6, false), "\n")
	if big == flowers || flowers == fruit {
		t.Errorf("plain trees look the same:\n%s\n\n%s\n\n%s", big, flowers, fruit)
	}
}
