package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// Search works on paragraphs, not screen lines, so a phrase that wraps
// still matches. Matching is "smart accent": a query typed without
// diacritics ignores them ("ban" finds bạn, bàn, bán); a query with
// diacritics must match them exactly. Case never matters.

type match struct{ from, to cellPos } // to is exclusive

type searchState struct {
	prompt   bool   // the "/" prompt is open
	query    string // as typed
	matches  []match
	cur      int
	savedTop int // position to restore when the prompt is cancelled
}

func (s *searchState) active() bool { return s.query != "" && !s.prompt && len(s.matches) > 0 }

// hasMarks reports whether q carries diacritics (or đ), which switches
// matching to accent-sensitive.
func hasMarks(q string) bool {
	return fold(q) != strings.ToLower(norm.NFC.String(q))
}

// cluster is a base rune with its combining marks, at a screen column.
type cluster struct {
	key       string // folded or normalized text used for matching
	line, col int
	width     int
}

// blockClusters splits the text of the lines [from, to) of one block into
// clusters, joining wrapped lines with a single space.
func (m *Model) blockClusters(from, to int, sensitive bool) []cluster {
	var out []cluster
	for li := from; li < to; li++ {
		l := m.doc.Lines[li]
		text := l.Plain()
		col, skip := 0, l.Gutter
		runes := []rune(text)
		// skip decoration and indentation at the start of the line
		i := 0
		for i < len(runes) && (col < skip || !l.Pre && (runes[i] == ' ' || runes[i] == '│')) {
			col += runewidth.RuneWidth(runes[i])
			i++
		}
		if len(out) > 0 {
			last := out[len(out)-1]
			out = append(out, cluster{key: " ", line: last.line, col: last.col + last.width})
		}
		for i < len(runes) {
			j := i + 1
			for j < len(runes) && unicode.Is(unicode.Mn, runes[j]) {
				j++
			}
			s := string(runes[i:j])
			key := strings.ToLower(norm.NFC.String(s))
			if !sensitive {
				key = fold(s)
			}
			w := runewidth.StringWidth(s)
			out = append(out, cluster{key: key, line: li, col: col, width: w})
			col += w
			i = j
		}
	}
	return out
}

// findMatches scans every text block for the query.
func (m *Model) findMatches(q string) []match {
	q = strings.TrimSpace(q)
	if q == "" || m.doc == nil {
		return nil
	}
	sensitive := hasMarks(q)
	needle := fold(q)
	if sensitive {
		needle = strings.ToLower(norm.NFC.String(q))
	}
	var out []match
	lines := m.doc.Lines
	for start := 0; start < len(lines); {
		l := lines[start]
		if l.Blank() || l.Deco || l.Image > 0 {
			start++
			continue
		}
		end := start + 1
		for end < len(lines) && !lines[end].Blank() && !lines[end].Deco && lines[end].Image == 0 &&
			lines[end].Chapter == l.Chapter && lines[end].Block == l.Block {
			end++
		}
		cs := m.blockClusters(start, end, sensitive)
		var hay strings.Builder
		offs := make([]int, 0, len(cs)+1) // byte offset where each cluster starts
		for _, c := range cs {
			offs = append(offs, hay.Len())
			hay.WriteString(c.key)
		}
		offs = append(offs, hay.Len())
		h := hay.String()
		for pos := 0; pos < len(h); {
			i := strings.Index(h[pos:], needle)
			if i < 0 {
				break
			}
			b, e := pos+i, pos+i+len(needle)
			first, last := clusterAt(offs, b), clusterAt(offs, e-1)
			if first >= 0 && last >= 0 {
				out = append(out, match{
					from: cellPos{cs[first].line, cs[first].col},
					to:   cellPos{cs[last].line, cs[last].col + cs[last].width},
				})
			}
			pos = e
		}
		start = end
	}
	return out
}

// clusterAt is the cluster containing byte offset b.
func clusterAt(offs []int, b int) int {
	for i := 0; i+1 < len(offs); i++ {
		if b >= offs[i] && b < offs[i+1] {
			return i
		}
	}
	return -1
}

// matchCols returns the columns of each match on line li, and whether the
// current match is among them.
func (m *Model) matchRanges(li int) (ranges [][2]int, current [][2]int) {
	if m.search.query == "" || len(m.search.matches) == 0 {
		return nil, nil
	}
	for i, mt := range m.search.matches {
		if mt.from.line > li {
			break
		}
		if mt.to.line < li {
			continue
		}
		c0, c1 := 0, 1<<30
		if mt.from.line == li {
			c0 = mt.from.col
		}
		if mt.to.line == li {
			c1 = mt.to.col
		}
		if i == m.search.cur {
			current = append(current, [2]int{c0, c1})
		} else {
			ranges = append(ranges, [2]int{c0, c1})
		}
	}
	return ranges, current
}

func (m *Model) openSearch() {
	m.search = searchState{prompt: true, savedTop: m.top, query: m.search.query}
	m.refreshSearch(false)
}

// refreshSearch recomputes matches; with jump, it moves to the first match
// at or after where the search started.
func (m *Model) refreshSearch(jump bool) {
	m.search.matches = m.findMatches(m.search.query)
	m.search.cur = 0
	for i, mt := range m.search.matches {
		if mt.from.line >= m.search.savedTop {
			m.search.cur = i
			break
		}
	}
	if jump {
		if len(m.search.matches) > 0 {
			m.showMatch()
		} else {
			m.top = m.search.savedTop
		}
	}
}

// showMatch scrolls so the current match is visible, a third down the screen.
func (m *Model) showMatch() {
	if len(m.search.matches) == 0 {
		return
	}
	line := m.search.matches[m.search.cur].from.line
	if line < m.top || line >= m.top+m.bodyH()-1 {
		m.top = line - m.bodyH()/3
		m.clamp()
	}
}

func (m *Model) stepMatch(d int) {
	n := len(m.search.matches)
	if n == 0 {
		return
	}
	m.search.cur = (m.search.cur + d + n) % n
	m.showMatch()
}

// searchKey edits the prompt. Every key is consumed while it's open.
func (m *Model) searchKey(k string) {
	switch k {
	case "esc":
		m.top = m.search.savedTop
		m.search = searchState{}
	case "enter":
		m.search.prompt = false
		if strings.TrimSpace(m.search.query) == "" {
			m.search = searchState{}
		}
	case "backspace":
		if r := []rune(m.search.query); len(r) > 0 {
			m.search.query = string(r[:len(r)-1])
			m.refreshSearch(true)
		}
	case "ctrl+u":
		m.search.query = ""
		m.refreshSearch(true)
	default:
		if utf8.RuneCountInString(k) == 1 {
			m.search.query += k
			m.refreshSearch(true)
		}
	}
}

// searchStatus is the status bar text while searching, or "".
func (m *Model) searchStatus() string {
	s := &m.search
	switch {
	case s.prompt:
		count := ""
		if s.query != "" {
			count = fmt.Sprintf("  %d found", len(s.matches))
		}
		return "/" + s.query + "▏" + count
	case s.query != "" && len(s.matches) == 0:
		return fmt.Sprintf("no matches for “%s” · esc", s.query)
	case s.active():
		return fmt.Sprintf("“%s” %d/%d · n N · esc", s.query, s.cur+1, len(s.matches))
	}
	return ""
}
