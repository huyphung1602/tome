package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/state"
)

func TestBookmarkToggleListJumpDelete(t *testing.T) {
	var saved state.Position
	m := New(testBook(), state.Position{HideTOC: true}, func(p state.Position) { saved = p })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})

	keys(m, runes("n")) // chapter 1
	_, cmd := m.Update(runes("B"))
	if len(m.bookmarks) != 1 || len(saved.Bookmarks) != 1 || cmd == nil || !strings.Contains(m.notice, "bookmarked") {
		t.Fatalf("B: bookmarks=%v saved=%v notice=%q", m.bookmarks, saved.Bookmarks, m.notice)
	}
	mark := m.bookmarks[0]
	if !strings.Contains(m.View(), "◆") {
		t.Error("no gutter mark")
	}

	keys(m, runes("g"))
	keys(m, runes("'"))
	v := m.View()
	if m.overlay != overlayBookmarks || !strings.Contains(v, "Chapter 1") {
		t.Fatalf("list not shown:\n%s", v)
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEnter})
	if l := m.bookmarkLine(mark); m.top > l || m.top < l-2 {
		t.Errorf("jump: top %d, bookmark line %d", m.top, l)
	}

	keys(m, runes("'"), runes("d"), tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.bookmarks) != 0 || len(saved.Bookmarks) != 0 {
		t.Errorf("delete: %v %v", m.bookmarks, saved.Bookmarks)
	}

	// restored from saved state
	m2 := New(testBook(), state.Position{Bookmarks: []state.Mark{mark}}, nil)
	m2.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if len(m2.bookmarks) != 1 {
		t.Error("bookmarks not loaded")
	}
}

func TestMarginClickBookmarksButDragSelects(t *testing.T) {
	var copied string
	m := New(testBook(), state.Position{HideTOC: true}, nil)
	m.SetClipboard(func(s string) error { copied = s; return nil })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m.View()
	_, y := screen(t, m, "c0 para 3")

	m.Update(mouse(m.contentX-1, y, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(m.contentX-1, y, tea.MouseButtonLeft, tea.MouseActionRelease))
	if len(m.bookmarks) != 1 {
		t.Fatalf("margin click: %v", m.bookmarks)
	}

	// a drag that starts in the margin selects text and leaves bookmarks alone
	x1, _ := screen(t, m, "para 3")
	drag(m, m.contentX-1, y, x1+4, y)
	if len(m.bookmarks) != 1 || !strings.HasPrefix(copied, "c0 para") {
		t.Errorf("drag from margin: bookmarks %v copied %q", m.bookmarks, copied)
	}
}

func TestBookmarksEmptyAndMenu(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	keys(m, runes("'"))
	if !strings.Contains(m.View(), "no bookmarks yet") {
		t.Error("missing empty-state hint")
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEsc}, runes("m"))
	v := m.View()
	if !strings.Contains(v, "search") || !strings.Contains(v, "bookmarks") {
		t.Errorf("menu lacks search/bookmarks:\n%s", v)
	}
}

func TestHelpFitsShortScreen(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	keys(m, runes("?"))
	v := m.View()
	for _, want := range []string{"esc close", "search", "bookmark", "page down"} {
		if !strings.Contains(v, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestGutterMarkWhenParagraphStartsAboveScreen(t *testing.T) {
	long := strings.Repeat("word ", 200)
	b := &epub.Book{Chapters: []epub.Chapter{{Path: "c.html", HTML: []byte("<p>" + long + "</p>")}}}
	m := New(b, state.Position{HideTOC: true}, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	keys(m, runes("jjjjj")) // into the middle of the paragraph
	keys(m, runes("B"))
	if !strings.Contains(m.View(), "◆") {
		t.Error("no mark when the paragraph began above the screen")
	}
}
