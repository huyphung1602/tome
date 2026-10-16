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
	if i := slices.Index(m.bookmarks, mk); i >= 0 {
		m.bookmarks = slices.Delete(m.bookmarks, i, i+1)
		m.persist()
		return "bookmark removed"
	}
	m.bookmarks = append(m.bookmarks, mk)
	slices.SortFunc(m.bookmarks, func(a, b state.Mark) int {
		if a.Chapter != b.Chapter {
			return a.Chapter - b.Chapter
		}
		return a.Block - b.Block
	})
	m.persist()
	return "bookmarked · ' to list"
}

// bookmarkLine is the first line of a bookmarked paragraph.
func (m *Model) bookmarkLine(mk state.Mark) int { return m.doc.Locate(mk.Chapter, mk.Block) }

// markedLines are the lines that get a gutter mark: the first visible
// line of each bookmarked paragraph, even when it started above the screen.
func (m *Model) markedLines() map[int]bool {
	out := map[int]bool{}
	if len(m.bookmarks) == 0 {
		return out
	}
	seen := map[state.Mark]bool{}
	for li := m.top; li < min(len(m.doc.Lines), m.top+m.bodyH()); li++ {
		l := m.doc.Lines[li]
		if l.Blank() || l.Deco {
			continue
		}
		mk := state.Mark{Chapter: l.Chapter, Block: l.Block}
		if !seen[mk] && m.isBookmarked(mk) {
			out[li] = true
		}
		seen[mk] = true
	}
	return out
}

func (m *Model) jumpToBookmark(i int) {
	if i < 0 || i >= len(m.bookmarks) {
		return
	}
	m.jump(max(0, m.bookmarkLine(m.bookmarks[i])-2)) // a little context above
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

// bookmarkRows renders the list for the bookmarks overlay.
func (m *Model) bookmarkRows(innerW int) []string {
	lines := []string{bold.Render("bookmarks"), ""}
	if len(m.bookmarks) == 0 {
		lines = append(lines, faint.Render("no bookmarks yet: press B on a paragraph,"), faint.Render("or click the left margin next to it"))
	}
	maxTop := max(1, len(m.doc.Lines)-m.bodyH())
	for _, mk := range m.bookmarks {
		li := m.bookmarkLine(mk)
		pct := fmt.Sprintf("%3d%%", min(100, li*100/maxTop))
		title := runewidth.Truncate(m.chapterTitleAt(li), innerW/3, "…")
		rest := innerW - runewidth.StringWidth(title) - 4 - 3 - 2
		row := bookmarkMark() + " " + bold.Render(title) + "  " + m.snippet(li, max(10, rest-1))
		pad := max(1, innerW-ansi.StringWidth(row)-len(pct))
		row += strings.Repeat(" ", pad) + faint.Render(pct)
		lines = append(lines, row)
	}
	hint := "enter jump · d delete · esc close"
	return append(lines, "", faint.Render(hint))
}
