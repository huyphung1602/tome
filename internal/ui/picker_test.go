package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	p := NewPicker(lib, recent, root, "", "")
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
	p := NewPicker(lib, nil, "/", "", "")
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
	p := NewPicker(lib, nil, root, "", "")
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if !strings.Contains(p.View(), "no EPUB files found") {
		t.Error("missing empty message")
	}
}
