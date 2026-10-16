package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/render"
)

// Text selection happens inside tome because the app captures the mouse:
// dragging selects by character, releasing copies clean text (paragraphs
// unwrapped, no margins or sidebar), double-click selects a paragraph.

type cellPos struct{ line, col int } // col is a display column in Line.Plain()

type selection struct {
	active, dragging bool
	anchor, head     cellPos
	pressAt          time.Time
	lastClick        time.Time // last press+release without a drag
	lastClickPos     cellPos
}

type noticeMsg struct{ id int }

// SetClipboard sets how selected text is copied.
func (m *Model) SetClipboard(copy func(string) error) { m.copyFn = copy }

// contentPos maps a screen cell to a document position, clamping to the
// reading column.
func (m *Model) contentPos(x, y int) cellPos {
	y = max(0, min(y, m.bodyH()-1))
	line := max(0, min(m.top+y, len(m.doc.Lines)-1))
	col := max(0, x-m.contentX)
	if w := runewidth.StringWidth(m.doc.Lines[line].Plain()); col > w {
		col = w
	}
	return cellPos{line, col}
}

func (m *Model) inContent(x, y int) bool {
	return y < m.bodyH() && x >= m.contentX-2 && x < m.contentX+m.colW+2
}

func (m *Model) selRange() (from, to cellPos, ok bool) {
	if !m.sel.active {
		return
	}
	from, to = m.sel.anchor, m.sel.head
	if to.line < from.line || to.line == from.line && to.col < from.col {
		from, to = to, from
	}
	return from, to, from != to
}

// lineSel returns the selected columns [c0, c1) of line li, or c0 >= c1.
func (m *Model) lineSel(li int) (c0, c1 int) {
	from, to, ok := m.selRange()
	if !ok || li < from.line || li > to.line {
		return 0, 0
	}
	c1 = runewidth.StringWidth(m.doc.Lines[li].Plain())
	if li == from.line {
		c0 = from.col
	}
	if li == to.line {
		c1 = min(c1, to.col)
	}
	return c0, c1
}

// renderLine draws spans, reversing the columns [c0, c1).
func renderLine(spans []render.Span, c0, c1 int) string {
	if c0 >= c1 {
		var b strings.Builder
		for _, s := range spans {
			b.WriteString(styleFor(s.Style).Render(s.Text))
		}
		return b.String()
	}
	var b strings.Builder
	col := 0
	for _, s := range spans {
		var run strings.Builder
		runSel := false
		flush := func() {
			if run.Len() == 0 {
				return
			}
			st := styleFor(s.Style)
			if runSel {
				st = st.Reverse(true)
			}
			b.WriteString(st.Render(run.String()))
			run.Reset()
		}
		for _, r := range s.Text {
			sel := col >= c0 && col < c1
			if sel != runSel {
				flush()
				runSel = sel
			}
			run.WriteRune(r)
			col += runewidth.RuneWidth(r)
		}
		flush()
	}
	return b.String()
}

// cutCols returns the part of s between display columns c0 and c1.
func cutCols(s string, c0, c1 int) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if col >= c0 && col < c1 {
			b.WriteRune(r)
		}
		col += runewidth.RuneWidth(r)
	}
	return b.String()
}

// selectedText joins the selection into clean text: wrapped lines of one
// paragraph are rejoined with spaces, paragraphs keep a blank line between
// them, code keeps its line breaks, and margins, blockquote bars and
// heading rules are dropped.
func (m *Model) selectedText() string {
	from, to, ok := m.selRange()
	if !ok {
		return ""
	}
	var out strings.Builder
	prev := [2]int{-1, -1}
	blank := false
	for li := from.line; li <= to.line; li++ {
		l := m.doc.Lines[li]
		if l.Blank() {
			blank = true
			continue
		}
		if l.Deco || l.Image > 0 {
			continue
		}
		c0, c1 := m.lineSel(li)
		seg := cutCols(l.Plain(), c0, c1)
		if !l.Pre {
			seg = strings.TrimSpace(seg)
			for strings.HasPrefix(seg, "│") {
				seg = strings.TrimSpace(strings.TrimPrefix(seg, "│"))
			}
		}
		if seg == "" {
			continue
		}
		block := [2]int{l.Chapter, l.Block}
		if out.Len() > 0 {
			switch {
			case block != prev && blank:
				out.WriteString("\n\n")
			case block != prev || l.Pre:
				out.WriteString("\n")
			default:
				out.WriteString(" ")
			}
		}
		out.WriteString(seg)
		prev, blank = block, false
	}
	return out.String()
}

// selectBlock selects every line of the paragraph at line li.
func (m *Model) selectBlock(li int) {
	l := m.doc.Lines[li]
	if l.Blank() {
		return
	}
	same := func(i int) bool {
		o := m.doc.Lines[i]
		return !o.Blank() && o.Chapter == l.Chapter && o.Block == l.Block
	}
	first, last := li, li
	for first > 0 && same(first-1) {
		first--
	}
	for last+1 < len(m.doc.Lines) && same(last+1) {
		last++
	}
	m.sel.active = true
	m.sel.anchor = cellPos{first, 0}
	m.sel.head = cellPos{last, runewidth.StringWidth(m.doc.Lines[last].Plain())}
}

// copySelection copies the selection and shows how much was copied.
func (m *Model) copySelection() tea.Cmd {
	text := m.selectedText()
	if text == "" || m.copyFn == nil {
		return nil
	}
	err := m.copyFn(text)
	n := utf8.RuneCountInString(text)
	if err != nil {
		// OSC 52 may still have worked; say so rather than claim failure
		return m.flash(fmt.Sprintf("copied %d chars (via terminal)", n))
	}
	return m.flash(fmt.Sprintf("copied %d chars", n))
}

// flash shows a short message in the status bar.
func (m *Model) flash(msg string) tea.Cmd {
	m.noticeID++
	id := m.noticeID
	m.notice = msg
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return noticeMsg{id} })
}

// selectMouse handles left-button drags in the reading column. It returns
// handled=false for events it doesn't own.
func (m *Model) selectMouse(msg tea.MouseMsg) (cmd tea.Cmd, handled bool) {
	switch {
	case m.sel.dragging && msg.Action == tea.MouseActionMotion:
		// dragging past the top or bottom edge scrolls
		if msg.Y <= 0 && m.top > 0 {
			m.top--
		} else if msg.Y >= m.bodyH()-1 {
			m.top++
			m.clamp()
		}
		m.sel.head = m.contentPos(msg.X, msg.Y)
		return nil, true
	case m.sel.dragging && msg.Action == tea.MouseActionRelease:
		m.sel.dragging = false
		if _, _, ok := m.selRange(); !ok {
			// a plain click: remember it so the next press can be a double-click
			m.sel.active = false
			m.sel.lastClick, m.sel.lastClickPos = m.sel.pressAt, m.sel.anchor
			return nil, true
		}
		return m.copySelection(), true
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.inContent(msg.X, msg.Y):
		p := m.contentPos(msg.X, msg.Y)
		now := time.Now()
		if now.Sub(m.sel.lastClick) < 400*time.Millisecond && p.line == m.sel.lastClickPos.line {
			m.sel.lastClick = time.Time{}
			m.selectBlock(p.line)
			return m.copySelection(), true
		}
		m.sel = selection{active: true, dragging: true, anchor: p, head: p, pressAt: now,
			lastClick: m.sel.lastClick, lastClickPos: m.sel.lastClickPos}
		return nil, true
	}
	return nil, false
}
