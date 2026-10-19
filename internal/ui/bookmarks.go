package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/state"
)

// bookmarkMark is the "◆" drawn for a bookmark: the theme's red, like a
// ribbon marker (dim when NO_COLOR is set).
func bookmarkMark() string {
	if noColor {
		return faint.Render("◆")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("◆")
}

// bookmarkCursorMark is the cursor sitting on a bookmark's mark.
func bookmarkCursorMark() string {
	if noColor {
		return bold.Render("◆")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("◆")
}

// cursorMark is the line cursor: the bookmark diamond, dimmed, so it reads
// as "B would bookmark this paragraph". Without color it's a hollow diamond
// so it can't be mistaken for a bookmark.
func cursorMark() string {
	if noColor {
		return faint.Render("◇")
	}
	return faint.Render("◆")
}

// Bookmarks mark paragraphs, the same unit the reading position uses, so
// they stay put when the text re-wraps.

// blockAt is the (chapter, block) of the first text line at or after li.
func (m *Model) blockAt(li int) (state.Mark, int, bool) {
	for i := max(0, li); i < len(m.doc.Lines); i++ {
		if l := m.doc.Lines[i]; !l.Blank() && !l.Deco {
			return state.Mark{Chapter: l.Chapter, Block: l.Block}, i, true
		}
	}
	return state.Mark{}, 0, false
}

func (m *Model) isBookmarked(mk state.Mark) bool { return slices.Contains(m.bookmarks, mk) }

// toggleBookmark adds or removes the bookmark on the paragraph at line li.
func (m *Model) toggleBookmark(li int) string {
	mk, _, ok := m.blockAt(li)
	if !ok {
		return ""
	}
	start, _ := m.blockSpan(mk)
	what := "“" + m.snippet(start, 40) + "”"
	if i := slices.Index(m.bookmarks, mk); i >= 0 {
		m.bookmarks = slices.Delete(m.bookmarks, i, i+1)
		m.persist()
		return "bookmark removed: " + what
	}
	m.bookmarks = append(m.bookmarks, mk)
	slices.SortFunc(m.bookmarks, func(a, b state.Mark) int {
		if a.Chapter != b.Chapter {
			return a.Chapter - b.Chapter
		}
		return a.Block - b.Block
	})
	m.persist()
	return "bookmarked: " + what + " · ' lists"
}

// bookmarkLine is the first line of a bookmarked paragraph.
func (m *Model) bookmarkLine(mk state.Mark) int { return m.doc.Locate(mk.Chapter, mk.Block) }

// gutterMarks maps screen lines to the mark drawn in the gutter: a red
// diamond on the first visible line of each bookmarked paragraph, and the
// dim cursor diamond on the cursor's line (a bold red one when the cursor
// is on a bookmark's mark, so B would remove it).
func (m *Model) gutterMarks() map[int]string {
	out := map[int]string{}
	seen := map[state.Mark]bool{}
	for li := m.top; li < min(len(m.doc.Lines), m.top+m.bodyH()); li++ {
		l := m.doc.Lines[li]
		if l.Blank() || l.Deco {
			continue
		}
		mk := state.Mark{Chapter: l.Chapter, Block: l.Block}
		if !seen[mk] && m.isBookmarked(mk) {
			out[li] = bookmarkMark()
		}
		seen[mk] = true
	}
	if m.caretOn && m.caret >= m.top && m.caret < m.top+m.bodyH() {
		if _, marked := out[m.caret]; marked {
			out[m.caret] = bookmarkCursorMark()
		} else {
			out[m.caret] = cursorMark()
		}
	}
	return out
}

// blockSpan is the [start, end) line range of the paragraph mk.
func (m *Model) blockSpan(mk state.Mark) (int, int) {
	start := m.doc.Locate(mk.Chapter, mk.Block)
	end := start
	for end < len(m.doc.Lines) {
		l := m.doc.Lines[end]
		if l.Chapter != mk.Chapter || l.Block != mk.Block {
			break
		}
		end++
	}
	return start, end
}

func (m *Model) jumpToBookmark(i int) {
	if i < 0 || i >= len(m.bookmarks) {
		return
	}
	li := m.bookmarkLine(m.bookmarks[i])
	m.jump(max(0, li-2)) // a little context above
	m.setCursor(li)
}

// chapterTitleAt is the TOC entry title that covers line li.
func (m *Model) chapterTitleAt(li int) string {
	title := ""
	for _, it := range m.toc {
		if it.line >= 0 && it.line <= li {
			title = it.title
		}
	}
	return title
}

// snippet is the start of the paragraph at line li.
func (m *Model) snippet(li, width int) string {
	var parts []string
	l0 := m.doc.Lines[li]
	for i := li; i < len(m.doc.Lines) && runewidth.StringWidth(strings.Join(parts, " ")) < width; i++ {
		l := m.doc.Lines[i]
		if l.Blank() || l.Chapter != l0.Chapter || l.Block != l0.Block {
			break
		}
		parts = append(parts, strings.Trim(l.Plain()[min(len(l.Plain()), l.Gutter):], " │"))
	}
	return runewidth.Truncate(strings.Join(parts, " "), width, "…")
}

func (m *Model) bookmarksKey(k string) {
	switch k {
	case "j", "down":
		m.bmSel = min(m.bmSel+1, len(m.bookmarks)-1)
	case "k", "up":
		m.bmSel = max(m.bmSel-1, 0)
	case "g", "home":
		m.bmSel = 0
	case "G", "end":
		m.bmSel = max(0, len(m.bookmarks)-1)
	case "pgdown", " ", "ctrl+d":
		m.bmSel = min(m.bmSel+m.bookmarkListH(), len(m.bookmarks)-1)
	case "pgup", "b", "ctrl+u":
		m.bmSel = max(m.bmSel-m.bookmarkListH(), 0)
	case "enter":
		m.overlay = overlayNone
		m.jumpToBookmark(m.bmSel)
	case "d", "x", "backspace", "delete":
		if m.bmSel < len(m.bookmarks) {
			m.bookmarks = slices.Delete(m.bookmarks, m.bmSel, m.bmSel+1)
			m.bmSel = max(0, min(m.bmSel, len(m.bookmarks)-1))
			m.persist()
		}
	case "esc", "'", "q":
		m.overlay = overlayNone
	}
}

// maxBookmarkRows caps the bookmark list; longer lists scroll.
const maxBookmarkRows = 15

// bookmarkListH is how many bookmarks the list shows at once: up to
// maxBookmarkRows, fewer on short screens so the box always fits.
func (m *Model) bookmarkListH() int {
	// box border 2 + title 2 + "more" rows 2 + blank and hint 2
	return max(3, min(maxBookmarkRows, m.h-8))
}

// bookmarkRows renders the bookmarks overlay: title with position, a
// window of the list that follows the selection, "more" markers above and
// below, and the key hint. Rows are fixed so clicks map to bookmarks.
func (m *Model) bookmarkRows(innerW int) []string {
	title := bold.Render("bookmarks")
	if n := len(m.bookmarks); n > 0 {
		title += faint.Render(fmt.Sprintf("  %d/%d", m.bmSel+1, n))
	}
	lines := []string{title, ""}
	if len(m.bookmarks) == 0 {
		lines = append(lines, faint.Render("no bookmarks yet: press B on a paragraph,"), faint.Render("or click the left margin next to it"))
		return append(lines, "", faint.Render("esc close"))
	}
	// keep the selection inside the visible window
	h := m.bookmarkListH()
	if m.bmSel < m.bmTop {
		m.bmTop = m.bmSel
	} else if m.bmSel >= m.bmTop+h {
		m.bmTop = m.bmSel - h + 1
	}
	m.bmTop = max(0, min(m.bmTop, len(m.bookmarks)-h))
	end := min(len(m.bookmarks), m.bmTop+h)

	more := func(n int, arrow string) string {
		if n <= 0 {
			return ""
		}
		return faint.Render(fmt.Sprintf("%s %d more", arrow, n))
	}
	lines = append(lines, more(m.bmTop, "↑"))
	maxTop := max(1, len(m.doc.Lines)-m.bodyH())
	for _, mk := range m.bookmarks[m.bmTop:end] {
		li := m.bookmarkLine(mk)
		pct := fmt.Sprintf("%3d%%", min(100, li*100/maxTop))
		title := runewidth.Truncate(m.chapterTitleAt(li), innerW/3, "…")
		rest := innerW - runewidth.StringWidth(title) - 4 - 3 - 2
		row := bookmarkMark() + " " + bold.Render(title) + "  " + m.snippet(li, max(10, rest-1))
		pad := max(1, innerW-ansi.StringWidth(row)-len(pct))
		row += strings.Repeat(" ", pad) + faint.Render(pct)
		lines = append(lines, row)
	}
	lines = append(lines, more(len(m.bookmarks)-end, "↓"))
	hint := "↑↓ select · enter jump · d delete · esc close"
	return append(lines, "", faint.Render(hint))
}
