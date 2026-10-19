package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/render"
	"github.com/huyphung1602/tome/internal/state"
)

func testBook() *epub.Book {
	b := &epub.Book{Title: "T"}
	for i := 0; i < 3; i++ {
		var sb strings.Builder
		fmt.Fprintf(&sb, "<h1>Chapter %d</h1>", i)
		for j := 0; j < 40; j++ {
			fmt.Fprintf(&sb, "<p>c%d para %d</p>", i, j)
		}
		p := fmt.Sprintf("c%d.html", i)
		b.Chapters = append(b.Chapters, epub.Chapter{Path: p, HTML: []byte(sb.String())})
		b.TOC = append(b.TOC, epub.TOCEntry{Title: fmt.Sprintf("Chapter %d", i), Path: p})
	}
	return b
}

func keys(m *Model, ks ...tea.KeyMsg) {
	for _, k := range ks {
		m.Update(k)
	}
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func topText(m *Model) string { return strings.TrimSpace(m.doc.Lines[m.top].Plain()) }

func TestBatchedRunesAreSeparateKeys(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	keys(m, runes("jjj"))
	if strings.TrimSpace(m.doc.Lines[m.caret].Plain()) != "c0 para 2" { // heading, rule skipped, para 0, 1, 2
		t.Errorf("caret on %q, want three text lines down", m.doc.Lines[m.caret].Plain())
	}
}

func TestChapterNavAndTOCJump(t *testing.T) {
	var saved state.Position
	m := New(testBook(), state.Position{}, func(p state.Position) { saved = p })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})

	keys(m, runes("n"))
	if topText(m) != "Chapter 1" || saved.Chapter != 1 {
		t.Fatalf("after n: %q saved=%+v", topText(m), saved)
	}
	keys(m, runes("p"))
	if topText(m) != "Chapter 0" {
		t.Fatalf("after p: %q", topText(m))
	}
	keys(m, tea.KeyMsg{Type: tea.KeyTab}, runes("jj"), tea.KeyMsg{Type: tea.KeyEnter})
	if topText(m) != "Chapter 2" || m.focusTOC {
		t.Fatalf("after toc jump: %q focus=%v", topText(m), m.focusTOC)
	}
	if !strings.Contains(m.View(), "▸ Chapter 2") {
		t.Error("current chapter not marked in sidebar")
	}
}

func TestRestoreSurvivesWidthChange(t *testing.T) {
	m := New(testBook(), state.Position{Chapter: 1, Block: 10}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	want := topText(m)
	if want != "c1 para 9" {
		t.Fatalf("restored to %q", want)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	if topText(m) != want {
		t.Errorf("after resize %q want %q", topText(m), want)
	}
}

func TestNarrowTerminalTOCOverlay(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	if m.colW > 56 {
		t.Errorf("column %d too wide for 60-col terminal", m.colW)
	}
	if strings.Contains(m.View(), "▸") {
		t.Error("TOC should be hidden on a narrow terminal until opened")
	}
	keys(m, runes("t"))
	if !m.focusTOC || !strings.Contains(m.View(), "Chapter 2") {
		t.Error("t should open the TOC overlay")
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.focusTOC || !m.showTOC {
		t.Error("esc should close overlay but keep the sidebar preference")
	}
}

func TestSaveFromBlankLineUsesNextBlock(t *testing.T) {
	var saved state.Position
	m := New(testBook(), state.Position{}, func(p state.Position) { saved = p })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m.top = 2 // blank line after the chapter heading and its rule
	if !m.doc.Lines[m.top].Blank() {
		t.Fatalf("line %d is not blank: %q", m.top, m.doc.Lines[m.top].Plain())
	}
	keys(m, runes("q"))
	m2 := New(testBook(), saved, nil)
	m2.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if topText(m2) != "c0 para 0" {
		t.Errorf("restored to %q", topText(m2))
	}
}

func mouse(x, y int, b tea.MouseButton, a tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: b, Action: a}
}

func TestLongTOCTitlesWrapNotTruncate(t *testing.T) {
	b := testBook()
	b.TOC[1].Title = "Mọi phiền muộn đều bắt nguồn từ quan hệ giữa người với người"
	m := New(b, state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	v := m.View()
	if strings.Contains(v, "…") {
		t.Error("TOC title truncated")
	}
	for _, w := range []string{"Mọi", "người với người"} {
		if !strings.Contains(v, w) {
			t.Errorf("missing %q in sidebar", w)
		}
	}
	// the wrapped entry spans several rows that all map to entry 1
	n := 0
	for _, idx := range m.rowItem {
		if idx == 1 {
			n++
		}
	}
	if n < 2 {
		t.Errorf("entry 1 uses %d rows, want wrapped", n)
	}
}

func TestMouseClickJumpsAndDragResizes(t *testing.T) {
	var saved state.Position
	m := New(testBook(), state.Position{}, func(p state.Position) { saved = p })
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	m.View()
	row := -1
	for y, idx := range m.rowItem {
		if idx == 2 {
			row = y
			break
		}
	}
	m.Update(mouse(m.tocX+5, m.textTop()+row, tea.MouseButtonLeft, tea.MouseActionPress))
	if topText(m) != "Chapter 2" {
		t.Fatalf("click jumped to %q", topText(m))
	}

	border := m.tocX + m.tocWidth() - 1
	m.Update(mouse(border, 3, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(border+10, 3, tea.MouseButtonNone, tea.MouseActionMotion))
	m.Update(mouse(border+10, 3, tea.MouseButtonNone, tea.MouseActionRelease))
	if m.sideW != defaultTOC+10 || saved.TOCWidth != defaultTOC+10 {
		t.Errorf("sideW=%d saved=%d", m.sideW, saved.TOCWidth)
	}
	// can't squeeze the reading area below minContent
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m.View()
	border = m.tocX + m.tocWidth() - 1
	m.Update(mouse(border, 3, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(95, 3, tea.MouseButtonNone, tea.MouseActionMotion))
	if m.sideW != 100-minContent {
		t.Errorf("sideW=%d want %d", m.sideW, 100-minContent)
	}
	m.Update(mouse(95, 3, tea.MouseButtonNone, tea.MouseActionRelease))
	keys(m, runes("<"))
	if m.sideW != 100-minContent-2 {
		t.Errorf("< gave %d", m.sideW)
	}
}

func TestWheelScrollsContent(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	m.View()
	m.Update(mouse(80, 5, tea.MouseButtonWheelDown, tea.MouseActionPress))
	if m.top != 3 {
		t.Errorf("top=%d", m.top)
	}
}

func TestHeadingColorsUseANSIPalette(t *testing.T) {
	if noColor {
		t.Skip("NO_COLOR set")
	}
	for level, want := range map[render.Style]string{render.H1: "5", render.H2: "4", render.H3: "6"} {
		if got := styleFor(render.Bold | level).GetForeground(); got != lipgloss.Color(want) {
			t.Errorf("level %b color = %v want %s", level, got, want)
		}
	}
	if _, ok := styleFor(render.Bold).GetForeground().(lipgloss.NoColor); !ok {
		t.Error("plain bold text should have no color")
	}
}

func TestBrowsingTOCKeepsWidth(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	m.View()
	w := m.sideW
	for i := 0; i < 10; i++ {
		m.Update(mouse(5, 5, tea.MouseButtonWheelDown, tea.MouseActionPress))
	}
	keys(m, tea.KeyMsg{Type: tea.KeyTab}, runes("jjjjjkkk"), tea.KeyMsg{Type: tea.KeyEsc})
	if m.sideW != w || m.tocWidth() != w {
		t.Errorf("width changed while browsing the TOC: %d -> %d", w, m.sideW)
	}
}

func TestSplitMouseSequencesAreNotKeys(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	w, top := m.sideW, m.top
	for _, frag := range []string{"<65;50;10M", "<64;50;10M", "65;50;10M", "<65;50", ";10M", "<0;12;3m"} {
		m.Update(runes(frag))
	}
	if m.sideW != w || m.top != top {
		t.Errorf("fragments acted as keys: width %d->%d top %d->%d", w, m.sideW, top, m.top)
	}
	// real keys still work, including a lone "<" and fast "jjj"
	caret := m.caret
	keys(m, runes("<"), runes("jjj"))
	if m.sideW != w-2 || m.caret == caret {
		t.Errorf("real keys: width %d caret %d", m.sideW, m.caret)
	}
}

func TestPickerIgnoresMouseFragments(t *testing.T) {
	lib, _ := scanned(t, "a.epub")
	p := NewPicker(lib, nil, "/", "", "", 0)
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p.Update(runes("<65;50;10M"))
	p.Update(runes("ab"))
	if p.query != "ab" {
		t.Errorf("query = %q", p.query)
	}
}
