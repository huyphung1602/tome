package ui

import (
	"fmt"
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

func TestCursorWithoutColorIsHollow(t *testing.T) {
	old := noColor
	noColor = true
	defer func() { noColor = old }()
	if cursorMark() == bookmarkMark() || !strings.Contains(cursorMark(), "◇") {
		t.Errorf("cursor %q vs bookmark %q", cursorMark(), bookmarkMark())
	}
}

func TestBookmarksListInBookOrderNotCreationOrder(t *testing.T) {
	m := New(testBook(), state.Position{HideTOC: true}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	keys(m, runes("G"), runes("B"))             // end of the book first
	keys(m, runes("g"), runes("}"), runes("B")) // then the start
	keys(m, runes("n"), runes("B"))             // then the middle
	var got []int
	for _, mk := range m.bookmarks {
		got = append(got, m.bookmarkLine(mk))
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("bookmarks out of book order: lines %v", got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("bookmarks = %v", got)
	}
	keys(m, runes("'"))
	v := m.View()
	if a, b := strings.Index(v, "Chapter 0"), strings.Index(v, "Chapter 2"); a < 0 || b < 0 || a > b {
		t.Errorf("list not in book order:\n%s", v)
	}
}

func manyBookmarks(t *testing.T, n int) *Model {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < n+20; i++ {
		fmt.Fprintf(&sb, "<p>para %d</p>", i)
	}
	b := &epub.Book{Chapters: []epub.Chapter{{Path: "c.html", HTML: []byte(sb.String())}}}
	var marks []state.Mark
	for i := 0; i < n; i++ {
		marks = append(marks, state.Mark{Chapter: 0, Block: i})
	}
	m := New(b, state.Position{Bookmarks: marks}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	keys(m, runes("'"))
	return m
}

func TestBookmarkListScrollsAndFits(t *testing.T) {
	m := manyBookmarks(t, 40)
	v := m.View()
	if m.hit.h > m.h || !strings.Contains(v, "1/40") || !strings.Contains(v, "↓ 25 more") || !strings.Contains(v, "enter jump") {
		t.Fatalf("box %d rows on a %d-row screen:\n%s", m.hit.h, m.h, v)
	}
	for i := 0; i < 35; i++ {
		keys(m, runes("j"))
	}
	v = m.View()
	if !strings.Contains(v, "para 35") || !strings.Contains(v, "36/40") || !strings.Contains(v, "↑ ") {
		t.Errorf("selection not scrolled into view:\n%s", v)
	}
	keys(m, runes("G"))
	m.View()
	if m.bmSel != 39 || !strings.Contains(m.View(), "para 39") {
		t.Error("G should select and show the last bookmark")
	}
	keys(m, runes("g"))
	if m.bmSel != 0 || m.View() == "" || m.bmTop != 0 {
		t.Error("g should go back to the first")
	}
	// wheel moves the selection
	m.Update(mouse(m.w/2, m.h/2, tea.MouseButtonWheelDown, tea.MouseActionPress))
	if m.bmSel != 3 {
		t.Errorf("wheel: sel %d", m.bmSel)
	}
}

func TestBookmarkListClickAfterScroll(t *testing.T) {
	m := manyBookmarks(t, 40)
	for i := 0; i < 30; i++ {
		keys(m, runes("j"))
	}
	m.View()
	// click the first visible bookmark row
	m.Update(mouse(m.hit.x+5, m.hit.y+4, tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayNone {
		t.Fatal("click didn't jump")
	}
	if got := strings.TrimSpace(m.doc.Lines[m.caret].Plain()); got != fmt.Sprintf("para %d", m.bmTop) {
		t.Errorf("jumped to %q, want para %d", got, m.bmTop)
	}
}
