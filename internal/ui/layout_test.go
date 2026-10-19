package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/state"
)

func TestSidebarAndTextCenteredTogether(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	m.View()
	if m.tocX == 0 {
		t.Fatal("sidebar should move in from the edge on a wide screen")
	}
	if gap := m.contentX - (m.tocX + m.tocWidth()); gap != sideGap+gutterW {
		t.Errorf("gap between sidebar and text = %d", gap)
	}
	leftSpace := m.tocX
	rightSpace := m.w - (m.contentX + m.colW)
	if d := leftSpace - rightSpace; d < -2 || d > 2 {
		t.Errorf("group not centered: %d left, %d right", leftSpace, rightSpace)
	}
}

func TestStatusLineAlignedWithColumn(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	m.View()
	st := m.topBar()
	plain := strings.TrimRight(stripANSI(st), " ")
	if lead := len(plain) - len(strings.TrimLeft(plain, " ")); lead != m.contentX {
		t.Errorf("status starts at %d, text at %d", lead, m.contentX)
	}
	if end := m.menuX + len([]rune(menuLabel)); end != m.contentX+m.colW {
		t.Errorf("menu ends at %d, column at %d", end, m.contentX+m.colW)
	}
	if !strings.Contains(plain, keysLabel) {
		t.Error("no ? keybinds button")
	}
	m.Update(mouse(m.keysX+1, m.barY(), tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayHelp {
		t.Error("? keybinds click should open help")
	}
}

func TestFirstRunHint(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.ShowHint()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.Init() == nil || !strings.Contains(m.status(), "press ? for keybinds") {
		t.Errorf("hint missing: %q", m.status())
	}
}

func TestPickerFramedAndRecentCapped(t *testing.T) {
	root := t.TempDir()
	var recent []state.Position
	for i := 0; i < 8; i++ {
		p := filepath.Join(root, fmt.Sprintf("book%d.epub", i))
		os.WriteFile(p, nil, 0o644)
		recent = append(recent, state.Position{Path: p, Title: fmt.Sprintf("Book %d", i), Progress: 10 + i})
	}
	lib, _ := scanned(t)
	p := NewPicker(lib, recent, root, "", "", 0)
	p.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	n := 0
	for _, e := range p.all {
		if e.recent {
			n++
		}
	}
	if n != maxRecent {
		t.Errorf("%d recent books, want %d", n, maxRecent)
	}
	v := p.View()
	rows := strings.Split(v, "\n")
	if p.frameTop() == 0 || strings.TrimSpace(rows[0]) != "" {
		t.Errorf("picker not centered vertically (top %d)", p.frameTop())
	}
	if !strings.Contains(v, "17%") { // Book 7, past the recent cap, keeps its progress
		t.Error("older recent book lost its progress")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestContentsButtonTogglesSidebar(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	m.View()
	if !strings.Contains(stripANSI(m.topBar()), tocLabel) || m.tocBtnX < 0 {
		t.Fatalf("no contents button: %q", stripANSI(m.topBar()))
	}
	m.Update(mouse(m.tocBtnX+1, m.barY(), tea.MouseButtonLeft, tea.MouseActionPress))
	if m.showTOC {
		t.Fatal("click did not hide the contents")
	}
	m.View()
	m.Update(mouse(m.tocBtnX+1, m.barY(), tea.MouseButtonLeft, tea.MouseActionPress))
	if !m.showTOC {
		t.Error("second click did not show the contents")
	}
	// narrow column: buttons drop from the left, the menu stays
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m.View()
	st := stripANSI(m.topBar())
	if strings.Contains(st, tocLabel) || !strings.Contains(st, menuLabel) {
		t.Errorf("narrow status = %q", st)
	}
}

func TestLongChapterTitleKeepsPercent(t *testing.T) {
	b := testBook()
	b.TOC[0].Title = strings.Repeat("Tâm lý học cá nhân và tổng thể luận ", 4)
	m := New(b, state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m.View()
	st := stripANSI(m.status())
	if !strings.Contains(st, "… · 0%") {
		t.Errorf("percent cut off: %q", st)
	}
}
