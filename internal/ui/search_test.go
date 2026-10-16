package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/state"
)

func viBook() *epub.Book {
	var sb strings.Builder
	sb.WriteString("<h1>Đêm thứ nhất</h1>")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&sb, "<p>đoạn %d chỉ là chữ lấp chỗ trống cho đủ dài.</p>", i)
	}
	sb.WriteString("<p>Bạn ấy ngồi bên cái bàn và bán sách cho người qua đường, một câu khá dài để phải xuống dòng.</p>")
	sb.WriteString("<p>Tôi BẬN lắm.</p>")
	// decomposed (NFD) text: "Dám" written as D a U+0301 m
	sb.WriteString("<p>Dám bị ghét</p>")
	return &epub.Book{Chapters: []epub.Chapter{{Path: "c.html", HTML: []byte(sb.String())}}}
}

func searchModel(t *testing.T) *Model {
	t.Helper()
	m := New(viBook(), state.Position{HideTOC: true}, nil)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	return m
}

func typeSearch(m *Model, q string) {
	keys(m, runes("/"))
	for _, r := range q {
		keys(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func matchedText(m *Model, mt match) string {
	var out []string
	for li := mt.from.line; li <= mt.to.line; li++ {
		c0, c1 := 0, 1<<30
		if li == mt.from.line {
			c0 = mt.from.col
		}
		if li == mt.to.line {
			c1 = mt.to.col
		}
		out = append(out, strings.TrimSpace(cutCols(m.doc.Lines[li].Plain(), c0, c1)))
	}
	return strings.Join(out, " ")
}

func TestSearchVietnameseSmartAccent(t *testing.T) {
	m := searchModel(t)
	// no accents typed: matches bạn, bàn, bán, BẬN
	got := map[string]bool{}
	for _, mt := range m.findMatches("ban") {
		got[strings.ToLower(matchedText(m, mt))] = true
	}
	for _, w := range []string{"bạn", "bàn", "bán", "bận"} {
		if !got[w] {
			t.Errorf("ban: missing %q in %v", w, got)
		}
	}
	// accents typed: only the exact word, any case
	if ms := m.findMatches("bận"); len(ms) != 1 || matchedText(m, ms[0]) != "BẬN" {
		t.Errorf("bận: %d matches", len(ms))
	}
	if ms := m.findMatches("bàn"); len(ms) != 1 {
		t.Errorf("bàn: %d matches", len(ms))
	}
	// đ without accent typed as d
	if ms := m.findMatches("dem thu nhat"); len(ms) != 1 {
		t.Errorf("dem thu nhat: %d matches", len(ms))
	}
	// decomposed text in the book still matches both ways
	if len(m.findMatches("dám bị")) != 1 || len(m.findMatches("dam bi")) != 1 {
		t.Error("NFD text did not match")
	}
}

func TestSearchPhraseAcrossWrappedLines(t *testing.T) {
	m := searchModel(t)
	ms := m.findMatches("người qua đường, một câu")
	if len(ms) != 1 {
		t.Fatalf("matches = %d", len(ms))
	}
	if ms[0].from.line == ms[0].to.line {
		t.Skip("phrase didn't wrap at this width")
	}
	if got := matchedText(m, ms[0]); got != "người qua đường, một câu" {
		t.Errorf("matched %q", got)
	}
}

func TestSearchPromptNavigateAndClear(t *testing.T) {
	m := searchModel(t)
	start := m.top
	typeSearch(m, "đoạn 2")
	if !m.search.prompt || len(m.search.matches) == 0 {
		t.Fatalf("prompt=%v matches=%d", m.search.prompt, len(m.search.matches))
	}
	if !strings.Contains(m.status(), "/đoạn 2") {
		t.Errorf("status = %q", m.status())
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEnter})
	n := len(m.search.matches) // đoạn 2, đoạn 20..29
	if n != 11 || !strings.Contains(m.status(), fmt.Sprintf("1/%d", n)) {
		t.Errorf("after enter: %d matches, status %q", n, m.status())
	}
	keys(m, runes("n"))
	if m.search.cur != 1 {
		t.Errorf("n -> cur %d", m.search.cur)
	}
	keys(m, runes("N"), runes("N"))
	if m.search.cur != n-1 {
		t.Errorf("N wraps -> cur %d", m.search.cur)
	}
	line := m.search.matches[m.search.cur].from.line
	if line < m.top || line >= m.top+m.bodyH() {
		t.Errorf("current match line %d not on screen (top %d)", line, m.top)
	}
	if hits, cur := m.matchRanges(line); len(cur) != 1 {
		t.Errorf("current match not highlighted: %v %v", hits, cur)
	}
	// esc clears; n goes back to chapters
	keys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.query != "" {
		t.Error("esc did not clear the search")
	}
	_ = start
}

func TestSearchCancelRestoresPosition(t *testing.T) {
	m := searchModel(t)
	keys(m, runes("jjj"))
	top := m.top
	typeSearch(m, "đường")
	if m.top == top {
		t.Fatal("incremental search didn't move")
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.top != top || m.search.query != "" {
		t.Errorf("cancel: top %d want %d, query %q", m.top, top, m.search.query)
	}
}

func TestSearchSurvivesResize(t *testing.T) {
	m := searchModel(t)
	typeSearch(m, "sách")
	keys(m, tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	if len(m.search.matches) != 1 || matchedText(m, m.search.matches[0]) != "sách" {
		t.Errorf("after resize: %+v", m.search.matches)
	}
}

func TestNoMatchesStatus(t *testing.T) {
	m := searchModel(t)
	typeSearch(m, "zzz")
	keys(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.status(), "no matches") {
		t.Errorf("status = %q", m.status())
	}
	keys(m, runes("n"))
	if m.search.cur != 0 {
		t.Error("n moved with no matches")
	}
}

func TestPagingWithHLAndArrows(t *testing.T) {
	m := searchModel(t)
	page := m.bodyH() - 2
	for _, k := range []tea.KeyMsg{runes("l"), {Type: tea.KeyRight}} {
		top := m.top
		m.Update(k)
		if m.top != top+page {
			t.Errorf("%v: top %d -> %d, want +%d", k, top, m.top, page)
		}
	}
	for _, k := range []tea.KeyMsg{runes("h"), {Type: tea.KeyLeft}} {
		top := m.top
		m.Update(k)
		if m.top != top-page {
			t.Errorf("%v: top %d -> %d, want -%d", k, top, m.top, page)
		}
	}
}
