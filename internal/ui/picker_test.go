package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/huyphung1602/tome/internal/library"
	"github.com/huyphung1602/tome/internal/state"
)

func scanned(t *testing.T, files ...string) (*library.Library, string) {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
	l := library.Scan(root)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, done := l.Snapshot(); done {
			break
		}
	}
	return l, root
}

func TestFoldStripsVietnameseDiacritics(t *testing.T) {
	if got := fold("Dám Bị Ghét — Đường"); got != "dam bi ghet — duong" {
		t.Errorf("fold = %q", got)
	}
}

func TestPickerSectionsFilterAndOpen(t *testing.T) {
	lib, root := scanned(t, "Dam Bi Ghet.epub", "classics/alice.epub", "classics/pride.epub")
	recent := []state.Position{{Path: filepath.Join(root, "classics/pride.epub"), Title: "Pride and Prejudice", Author: "Jane Austen", Progress: 15}}
	p := NewPicker(lib, recent, root, "", "", 0)
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	v := p.View()
	for _, want := range []string{"RECENT", "ALL BOOKS", "Pride and Prejudice", "Jane Austen", "15%", "alice", "3 books"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Count(v, "Pride and Prejudice") != 1 {
		t.Error("recent book listed twice")
	}

	// typing without diacritics filters, sections disappear
	for _, r := range "dam bi" {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(p.shown) != 1 || !strings.HasSuffix(p.shown[0].path, "Dam Bi Ghet.epub") {
		t.Fatalf("filtered = %+v", p.shown)
	}
	if strings.Contains(p.View(), "RECENT") {
		t.Error("section headers shown while filtering")
	}

	// esc clears the query first, then quits
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.query != "" || len(p.shown) != 3 {
		t.Fatalf("esc should clear query: %q %d", p.query, len(p.shown))
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil || p.Chosen != "" {
		t.Error("second esc should quit without choosing")
	}

	// the cursor stays on the book found by the filter; enter opens the selection
	if !strings.HasSuffix(p.shown[p.cursor].path, "Dam Bi Ghet.epub") {
		t.Errorf("cursor moved off the filtered book: %q", p.shown[p.cursor].path)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	want := p.shown[p.cursor].path
	_, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || p.Chosen != want {
		t.Errorf("chosen = %q", p.Chosen)
	}
}

func TestPickerClickOpens(t *testing.T) {
	lib, _ := scanned(t, "a.epub", "b.epub")
	p := NewPicker(lib, nil, "/", "", "", 0)
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p.View()
	row := -1
	for r, idx := range p.rowItem {
		if idx == 1 {
			row = r
		}
	}
	p.Update(mouse(10, row+3, tea.MouseButtonLeft, tea.MouseActionPress))
	if p.Chosen != p.shown[1].path {
		t.Errorf("click chose %q", p.Chosen)
	}
}

func TestPickerEmpty(t *testing.T) {
	lib, root := scanned(t)
	p := NewPicker(lib, nil, root, "", "", 0)
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if !strings.Contains(p.View(), "no EPUB files found") {
		t.Error("missing empty message")
	}
}
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

func TestPickerNarrowFitsStrip(t *testing.T) {
	p := gardenPicker(t, 40)
	p.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	for _, line := range strings.Split(p.View(), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line %d wide on a 50-column screen: %q", w, ansi.Strip(line))
		}
	}
}
