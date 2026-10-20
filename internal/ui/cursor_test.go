package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/state"
)

func vimModel(t *testing.T) *Model {
	t.Helper()
	m := New(testBook(), state.Position{HideTOC: true}, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 21}) // 20 body rows
	return m
}

func caretText(m *Model) string { return strings.TrimSpace(m.doc.Lines[m.caret].Plain()) }

func TestJKMoveCursorAndScrollOnlyAtEdge(t *testing.T) {
	m := vimModel(t)
	if m.caret != 0 {
		t.Fatalf("cursor starts at %d", m.caret)
	}
	keys(m, runes("jjj"))
	if m.top != 0 || !m.isText(m.caret) || m.caret == 0 {
		t.Fatalf("j should move the cursor, not the view: top %d caret %d", m.top, m.caret)
	}
	// keep going: the view starts scrolling with scrolloff lines left
	for i := 0; i < 30; i++ {
		keys(m, runes("j"))
	}
	so := m.scrolloff()
	if m.top == 0 || m.caret != m.top+m.bodyH()-1-so && !m.isText(m.top+m.bodyH()-1-so) {
		t.Errorf("after scrolling: top %d caret %d (scrolloff %d)", m.top, m.caret, so)
	}
	if m.caret > m.top+m.bodyH()-1-so {
		t.Errorf("cursor %d closer than scrolloff to the bottom (top %d)", m.caret, m.top)
	}
	keys(m, runes("k"))
	if m.caret < m.top {
		t.Error("k moved the cursor off screen")
	}
}

func TestWheelKeepsCursorUntilItWouldLeave(t *testing.T) {
	m := vimModel(t)
	m.View()
	keys(m, runes("jjjjjjjjjj")) // cursor mid-screen
	line, top := caretText(m), m.top
	m.Update(mouse(50, 5, tea.MouseButtonWheelDown, tea.MouseActionPress))
	if caretText(m) != line || m.top != top+3 {
		t.Errorf("wheel: cursor moved to %q (want %q), top %d", caretText(m), line, m.top)
	}
	for i := 0; i < 10; i++ {
		m.Update(mouse(50, 5, tea.MouseButtonWheelDown, tea.MouseActionPress))
	}
	if m.caret < m.top+m.scrolloff() {
		t.Errorf("cursor %d left behind above the view (top %d)", m.caret, m.top)
	}
}

func TestHalfPageMovesBothFullPageResetsCursor(t *testing.T) {
	m := vimModel(t)
	keys(m, runes("jjjjj"))
	rel := m.caret - m.top
	keys(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.top == 0 || m.caret-m.top < rel-1 || m.caret-m.top > rel+1 {
		t.Errorf("ctrl+d: top %d, cursor row %d want ~%d", m.top, m.caret-m.top, rel)
	}
	keys(m, runes(" "))
	if row := m.caret - m.top; row > m.scrolloff()+1 {
		t.Errorf("page down: cursor at row %d, want the top of the page", row)
	}
	keys(m, runes("b"))
	if row := m.caret - m.top; row < m.bodyH()-1-m.scrolloff()-1 {
		t.Errorf("page up: cursor at row %d, want the bottom of the page", row)
	}
	for _, k := range []tea.KeyMsg{runes("l"), {Type: tea.KeyRight}, runes("h"), {Type: tea.KeyLeft}} {
		top := m.top
		m.Update(k)
		if m.top == top {
			t.Errorf("%v did not page", k)
		}
	}
}

func TestBookmarkUsesCursorParagraph(t *testing.T) {
	m := vimModel(t)
	keys(m, runes("jjjjjj"))
	want := caretText(m)
	keys(m, runes("B"))
	if len(m.bookmarks) != 1 {
		t.Fatal("no bookmark")
	}
	start, _ := m.blockSpan(m.bookmarks[0])
	if got := strings.TrimSpace(m.doc.Lines[start].Plain()); got != want {
		t.Errorf("bookmarked %q, cursor on %q", got, want)
	}
	if !strings.Contains(m.View(), "◆") {
		t.Error("no mark drawn")
	}
	keys(m, runes("B"))
	if len(m.bookmarks) != 0 {
		t.Error("B on the cursor's bookmarked paragraph should remove it")
	}
}

func TestParagraphMotionAndJumpsMoveCursor(t *testing.T) {
	m := vimModel(t)
	keys(m, runes("}"))
	after := caretText(m)
	if after == "Chapter 0" || after == "" {
		t.Fatalf("} went to %q", after)
	}
	keys(m, runes("}"), runes("{"))
	if caretText(m) != after {
		t.Errorf("{ went to %q, want %q", caretText(m), after)
	}
	keys(m, runes("n"))
	if caretText(m) != "Chapter 1" {
		t.Errorf("chapter jump: cursor on %q", caretText(m))
	}
	keys(m, runes("G"))
	if m.caret != m.snap(len(m.doc.Lines)-1, -1) {
		t.Error("G should put the cursor on the last line")
	}
	keys(m, runes("g"))
	if m.caret != 0 || m.top != 0 {
		t.Error("g should go to the first line")
	}
}

func TestClickMovesCursor(t *testing.T) {
	m := vimModel(t)
	m.View()
	x, y := screen(t, m, "c0 para 5")
	m.Update(mouse(x+2, y, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(x+2, y, tea.MouseButtonLeft, tea.MouseActionRelease))
	if caretText(m) != "c0 para 5" || len(m.bookmarks) != 0 {
		t.Errorf("click: cursor on %q, bookmarks %v", caretText(m), m.bookmarks)
	}
}

func caretShown(m *Model) bool {
	return strings.Contains(m.View(), cursorMark()) || strings.Contains(m.View(), bookmarkCursorMark())
}

func TestCursorHidesWhenIdleAndOnlyAimingShowsIt(t *testing.T) {
	m := vimModel(t)
	if caretShown(m) {
		t.Fatal("cursor shown before any move")
	}
	_, cmd := m.Update(runes("j"))
	if !caretShown(m) || cmd == nil {
		t.Fatal("j should show the cursor and schedule hiding it")
	}
	m.Update(caretIdleMsg{id: m.caretID})
	if caretShown(m) {
		t.Fatal("cursor still shown after the idle tick")
	}
	// reading moves keep it hidden
	for _, k := range []tea.Msg{runes(" "), runes("b"), runes("l"), tea.KeyMsg{Type: tea.KeyCtrlD},
		mouse(50, 5, tea.MouseButtonWheelDown, tea.MouseActionPress)} {
		m.Update(k)
		if caretShown(m) {
			t.Errorf("%v revealed the cursor", k)
		}
	}
	// a stale tick from an earlier reveal doesn't hide a newer one
	old := m.caretID
	keys(m, runes("}"))
	m.Update(caretIdleMsg{id: old})
	if !caretShown(m) {
		t.Error("stale idle tick hid the cursor")
	}
	// jumps show it
	m.Update(caretIdleMsg{id: m.caretID})
	keys(m, runes("n"))
	if !caretShown(m) {
		t.Error("chapter jump should show the cursor")
	}
}

func TestBookmarkNoticeNamesParagraphAndShowsCursor(t *testing.T) {
	m := vimModel(t)
	keys(m, runes("jjjj"))
	m.Update(caretIdleMsg{id: m.caretID})
	keys(m, runes("B"))
	if !strings.Contains(m.notice, "bookmarked: “c0 para") {
		t.Errorf("notice = %q", m.notice)
	}
	if !strings.Contains(m.View(), bookmarkCursorMark()) {
		t.Error("B should show the cursor on the new bookmark")
	}
	keys(m, runes("B"))
	if !strings.HasPrefix(m.notice, "bookmark removed: “") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestPagesDontOverlap(t *testing.T) {
	m := vimModel(t)
	for i := 0; i < 3; i++ {
		last := m.top + m.bodyH() - 1 // last line on screen
		keys(m, runes(" "))
		if m.top != last+1 {
			t.Fatalf("page %d starts at line %d, previous page ended at %d", i+1, m.top, last)
		}
		if row := m.caret - m.top; row < 0 || row > m.scrolloff()+1 {
			t.Errorf("cursor at row %d after paging, want the top", row)
		}
	}
	top := m.top
	keys(m, runes("b"))
	if m.top != top-m.bodyH() {
		t.Errorf("page back: top %d want %d", m.top, top-m.bodyH())
	}
}
