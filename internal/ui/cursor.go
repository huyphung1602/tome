package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/state"
)

// The cursor is a line, moved like in vim: j/k move it and the view only
// scrolls when it gets within scrolloff lines of an edge; the mouse wheel
// scrolls the view and drags the cursor along only when its line would
// leave the screen; half pages move both; full pages put it at the top
// (or bottom) of the new page. B bookmarks the paragraph it's in.

// The cursor mark hides after caretIdle without aiming at a spot: moving
// the cursor, jumping, clicking or bookmarking shows it again; reading
// moves (wheel, half and full pages) don't.
const caretIdle = 3 * time.Second

type caretIdleMsg struct{ id int }

func (m *Model) showCaret() tea.Cmd {
	m.caretOn = true
	m.caretID++
	id := m.caretID
	return tea.Tick(caretIdle, func(time.Time) tea.Msg { return caretIdleMsg{id} })
}

// scrolloff is how close to the top or bottom edge the cursor may get.
func (m *Model) scrolloff() int { return min(3, max(0, (m.bodyH()-1)/4)) }

func (m *Model) isText(li int) bool {
	l := m.doc.Lines[li]
	return !l.Blank() && !l.Deco
}

// snap moves line li to the nearest text line, searching in direction d
// first (blank lines and rules can't hold the cursor).
func (m *Model) snap(li, d int) int {
	n := len(m.doc.Lines)
	if n == 0 {
		return 0
	}
	li = max(0, min(li, n-1))
	if d == 0 {
		d = 1
	}
	for _, dir := range []int{d, -d} {
		for i := li; i >= 0 && i < n; i += dir {
			if m.isText(i) {
				return i
			}
		}
	}
	return li
}

// keepCursorOnScreen drags the cursor into the visible window after the
// view scrolled, the way vim does for the mouse wheel.
func (m *Model) keepCursorOnScreen() {
	so := m.scrolloff()
	lo, hi := m.top+so, m.top+m.bodyH()-1-so
	if m.top == 0 {
		lo = 0
	}
	if m.top >= len(m.doc.Lines)-m.bodyH() {
		hi = len(m.doc.Lines) - 1
	}
	switch {
	case m.caret < lo:
		m.caret = m.snap(lo, 1)
	case m.caret > hi:
		m.caret = m.snap(hi, -1)
	}
}

// scrollToCursor scrolls just enough to keep scrolloff lines around it.
func (m *Model) scrollToCursor() {
	so := m.scrolloff()
	if m.caret < m.top+so {
		m.top = m.caret - so
	} else if m.caret > m.top+m.bodyH()-1-so {
		m.top = m.caret - (m.bodyH() - 1 - so)
	}
	m.clamp()
}

// moveLine moves the cursor n text lines (negative is up).
func (m *Model) moveLine(n int) {
	d := 1
	if n < 0 {
		d, n = -1, -n
	}
	for ; n > 0; n-- {
		next := m.caret + d
		for next >= 0 && next < len(m.doc.Lines) && !m.isText(next) {
			next += d
		}
		if next < 0 || next >= len(m.doc.Lines) {
			break
		}
		m.caret = next
	}
	m.scrollToCursor()
	m.reveal = true
}

// scrollView moves the view, leaving the cursor on its line if it stays visible.
func (m *Model) scrollView(n int) {
	m.top += n
	m.clamp()
	m.keepCursorOnScreen()
}

// halfPage moves view and cursor together (ctrl+d / ctrl+u).
func (m *Model) halfPage(d int) {
	n := d * max(1, m.bodyH()/2)
	m.top += n
	m.clamp()
	m.caret = m.snap(m.caret+n, d)
	m.keepCursorOnScreen()
}

// page turns a full page; the cursor goes to the top of the new page, or
// the bottom when paging back.
func (m *Model) page(d int) {
	m.top += d * max(1, m.bodyH()-2)
	m.clamp()
	so := m.scrolloff()
	if d > 0 {
		m.caret = m.snap(m.top+so, 1)
	} else {
		m.caret = m.snap(m.top+m.bodyH()-1-so, -1)
	}
	if m.top == 0 && d < 0 {
		m.caret = m.snap(min(m.caret, m.top+m.bodyH()-1), -1)
	}
	m.keepCursorOnScreen()
}

// setCursor puts the cursor on line li (or the next text line) after a
// jump or click. Like vim, scrolloff isn't enforced here, so a chapter
// heading at the top of the screen keeps the cursor.
func (m *Model) setCursor(li int) {
	m.caret = m.snap(li, 1)
	m.reveal = true
	if m.caret < m.top || m.caret >= m.top+m.bodyH() {
		m.keepCursorOnScreen()
	}
}

// paragraph motion: } to the start of the next paragraph, { to the start
// of this one, or of the previous one when already at its start.
func (m *Model) paragraphMotion(d int) {
	cur := m.caretPara()
	start, end := m.blockSpan(cur)
	if d > 0 {
		if _, li, ok := m.blockAt(end); ok {
			m.caret = li
		}
	} else if m.caret > start {
		m.caret = start
	} else {
		for i := start - 1; i >= 0; i-- {
			if m.isText(i) {
				l := m.doc.Lines[i]
				m.caret, _ = m.blockSpan(state.Mark{Chapter: l.Chapter, Block: l.Block})
				break
			}
		}
	}
	m.scrollToCursor()
	m.reveal = true
}

// caretPara is the paragraph the cursor is in.
func (m *Model) caretPara() state.Mark {
	if m.caret >= len(m.doc.Lines) {
		return state.Mark{}
	}
	l := m.doc.Lines[m.caret]
	return state.Mark{Chapter: l.Chapter, Block: l.Block}
}
