package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/state"
)

func tallModel(t *testing.T, pos state.Position) (*Model, *state.Position) {
	t.Helper()
	saved := &state.Position{}
	m := New(testBook(), pos, func(p state.Position) { *saved = p })
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	return m, saved
}

func TestReadingFrameCappedAndCentered(t *testing.T) {
	m, _ := tallModel(t, state.Position{})
	if m.bodyH() != defaultHeight {
		t.Fatalf("bodyH = %d want %d", m.bodyH(), defaultHeight)
	}
	rows := strings.Split(m.View(), "\n")
	if len(rows) != 60 {
		t.Fatalf("view has %d rows", len(rows))
	}
	top := m.frameTop()
	if top != (60-defaultHeight-2-2*statusGap)/2 || strings.TrimSpace(rows[top-1]) != "" {
		t.Errorf("frame top %d, row above = %q", top, rows[top-1])
	}
	if !strings.Contains(rows[m.barY()], "● menu") || !strings.Contains(rows[m.barY()], keysLabel) {
		t.Errorf("button bar = %q", rows[m.barY()])
	}
	for i := 1; i <= statusGap; i++ {
		if strings.TrimSpace(rows[m.barY()+i]) != "" {
			t.Errorf("row %d under the bar isn't blank", i)
		}
	}
	if !strings.Contains(rows[m.textTop()], "Chapter 0") {
		t.Errorf("first text row = %q", rows[m.textTop()])
	}
	for i := 1; i <= statusGap; i++ {
		if strings.TrimSpace(rows[m.statusY()-i]) != "" {
			t.Errorf("row %d above the status isn't blank: %q", i, rows[m.statusY()-i])
		}
	}
	if !strings.Contains(rows[m.statusY()], "Chapter 0 · 0%") {
		t.Errorf("status row %d = %q", m.statusY(), rows[m.statusY()])
	}
	// paging moves by the frame, not the screen
	keys(m, runes(" "))
	if m.top != defaultHeight { // pages don't overlap
		t.Errorf("page = %d lines", m.top)
	}
}

func TestMouseUsesFrameCoordinates(t *testing.T) {
	m, _ := tallModel(t, state.Position{})
	m.View()
	top := m.textTop()
	// TOC entry for chapter 2
	row := -1
	for y, idx := range m.rowItem {
		if idx == 2 {
			row = y
		}
	}
	m.Update(mouse(m.tocX+4, top+row, tea.MouseButtonLeft, tea.MouseActionPress))
	if topText(m) != "Chapter 2" {
		t.Errorf("TOC click landed on %q", topText(m))
	}
	// menu button on the status line, menu box right above it
	m.Update(mouse(m.menuX+1, m.barY(), tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayMenu {
		t.Fatal("menu button click missed")
	}
	m.View()
	if m.hit.y != m.barY()+2 { // box border is one row below the bar
		t.Errorf("menu items start at row %d, bar at %d", m.hit.y, m.barY())
	}
	// clicks on the bar row away from the buttons and in the gap do nothing
	m.overlay = overlayNone
	m.Update(mouse(m.contentX+2, m.barY(), tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(m.contentX+2, m.barY()+1, tea.MouseButtonLeft, tea.MouseActionPress))
	m.overlay = overlayNone
	// a click in the text moves the cursor to that line
	m.View()
	x, y := screen(t, m, "c2 para 4")
	m.Update(mouse(x+1, y, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(x+1, y, tea.MouseButtonLeft, tea.MouseActionRelease))
	if caretText(m) != "c2 para 4" {
		t.Errorf("click put the cursor on %q", caretText(m))
	}
}

func TestResizeReadingHeight(t *testing.T) {
	m, saved := tallModel(t, state.Position{})
	keys(m, runes("-"), runes("-"))
	if m.bodyH() != defaultHeight-4 || saved.ReadHeight != defaultHeight-4 {
		t.Errorf("after --: bodyH %d saved %d", m.bodyH(), saved.ReadHeight)
	}
	if !strings.Contains(m.status(), "reading height") {
		t.Errorf("status = %q", m.status())
	}
	for i := 0; i < 20; i++ {
		keys(m, runes("+"))
	}
	if m.maxH != fullHeight || m.bodyH() != 60-2-2*statusGap || m.frameTop() != 0 {
		t.Errorf("full height: maxH %d bodyH %d top %d", m.maxH, m.bodyH(), m.frameTop())
	}
	for i := 0; i < 40; i++ {
		keys(m, runes("-"))
	}
	if m.bodyH() != minHeight {
		t.Errorf("min height: %d", m.bodyH())
	}
	// restored from prefs; a short terminal still uses all of its rows
	m2, _ := tallModel(t, state.Position{ReadHeight: 20})
	if m2.bodyH() != 20 {
		t.Errorf("restored bodyH %d", m2.bodyH())
	}
	m2.Update(tea.WindowSizeMsg{Width: 140, Height: 15})
	if m2.bodyH() != 15-2-2*statusGap || m2.frameTop() != 0 {
		t.Errorf("short terminal: bodyH %d top %d", m2.bodyH(), m2.frameTop())
	}
}
