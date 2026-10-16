// Package ui is the Bubble Tea reader: a TOC sidebar and a centered,
// fixed-width reading column scrolled as one continuous page.
package ui

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/kitty"
	"github.com/huyphung1602/tome/internal/render"
	"github.com/huyphung1602/tome/internal/state"
)

const (
	maxColumn  = 72
	minContent = 64 // terminal columns the reading area needs beside the sidebar
	defaultTOC = 36 // sidebar width including the right border
	minTOC     = 20
	maxTOC     = 70
)

type tocItem struct {
	title string
	depth int
	path  string
	frag  string
	line  int
}

type Model struct {
	book *epub.Book
	doc  *render.Doc
	toc  []tocItem
	save func(state.Position)

	w, h     int
	colW     int
	top      int
	restore  *state.Position
	showTOC  bool
	focusTOC bool
	cursor   int
	tocTop   int  // first visible TOC entry
	lastSel  int  // selection the sidebar last scrolled to; -2 forces a re-scroll
	sideW    int  // sidebar width including the border
	dragging bool // dragging the sidebar border with the mouse
	rowItem  []int
	tocX     int // screen column where TOC entries start
	tocW     int

	overlay      overlayKind
	menuSel      int
	img          imageView
	hit          rect      // screen area of the open overlay box
	gfx          io.Writer // kitty graphics output; nil disables images
	cellW, cellH int       // terminal cell size in pixels
	inline       bool      // draw images in the reading column
	layoutH      int       // height the doc was laid out for (inline images depend on it)
	imgSize      map[string][2]int
	sent         map[int][2]int // inline image index -> cols, rows transmitted
	quitAll      bool           // ctrl+c: leave the app, not just this book
	css          render.CSS
	bookList     bool // opened from the book picker: q goes back to it

	sel      selection
	copyFn   func(string) error
	notice   string // brief status message, e.g. "copied 312 chars"
	noticeID int
	contentX int // screen column where the reading column starts
}

// isMouseFragment spots the tail of a mouse escape sequence (e.g.
// "<65;50;10M") that arrived split across reads, which Bubble Tea then
// reports as typed keys. Fast wheel scrolling, especially through a
// multiplexer, causes this; without the check its "<" shrinks the sidebar.
func isMouseFragment(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) < 2 {
		return false
	}
	semi := false
	for _, r := range msg.Runes {
		switch {
		case r == ';':
			semi = true
		case r >= '0' && r <= '9', r == '<', r == '[', r == 'M', r == 'm':
		default:
			return false
		}
	}
	return semi || msg.Runes[0] == '<' && msg.Runes[1] >= '0' && msg.Runes[1] <= '9'
}

// SetBookList marks the book as opened from the picker, so q returns to it.
func (m *Model) SetBookList(on bool) { m.bookList = on }

// QuitAll reports whether the user asked to leave the whole app (ctrl+c)
// rather than close the book (q).
func (m *Model) QuitAll() bool { return m.quitAll }

func New(book *epub.Book, pos state.Position, save func(state.Position)) *Model {
	m := &Model{book: book, save: save, showTOC: !pos.HideTOC, restore: &pos, lastSel: -2}
	m.sideW = defaultTOC
	if pos.TOCWidth > 0 {
		m.sideW = max(minTOC, min(maxTOC, pos.TOCWidth))
	}
	for _, e := range book.TOC {
		m.toc = append(m.toc, tocItem{title: e.Title, depth: e.Depth, path: e.Path, frag: e.Fragment})
	}
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) bodyH() int { return max(1, m.h-1) }

// tocWidth is the sidebar width that fits now: the preferred width,
// shrunk so the reading area keeps minContent columns.
func (m *Model) tocWidth() int { return min(m.sideW, m.w-minContent) }

func (m *Model) sideBySide() bool { return m.tocWidth() >= minTOC }

// layout re-renders the book when the usable column width changes,
// keeping the reading position anchored to its source block.
func (m *Model) layout() {
	avail := m.w
	if m.showTOC && m.sideBySide() {
		avail -= m.tocWidth()
	}
	colW := max(20, min(maxColumn, avail-4))
	if colW == m.colW && m.doc != nil && (!m.inline || m.layoutH == m.h) {
		return
	}
	pos := m.restore
	if pos == nil && m.doc != nil {
		p := m.position()
		pos = &p
	}
	m.restore = nil
	m.colW = colW
	m.layoutH = m.h
	if m.css == nil {
		m.css = render.ParseCSS(m.book.CSS)
	}
	opts := render.Options{Width: colW, CSS: m.css}
	if m.inline {
		opts.ImageFit = m.fitInline
	}
	m.doc = render.RenderWith(m.book.Chapters, opts)

	if len(m.book.TOC) == 0 {
		m.toc = m.toc[:0]
		for i, ch := range m.book.Chapters {
			title := m.doc.ChapterTitles[i]
			if title == "" {
				title = strings.TrimSuffix(path.Base(ch.Path), path.Ext(ch.Path))
			}
			m.toc = append(m.toc, tocItem{title: title, path: ch.Path})
		}
	}
	for i := range m.toc {
		m.toc[i].line = m.doc.Anchor(m.toc[i].path, m.toc[i].frag)
	}
	if pos != nil {
		m.top = m.doc.Locate(pos.Chapter, pos.Block)
	}
	m.clamp()
	m.cursor = max(0, m.current())
}

func (m *Model) position() state.Position {
	p := state.Position{HideTOC: !m.showTOC}
	if m.sideW != defaultTOC {
		p.TOCWidth = m.sideW
	}
	if m.doc != nil && m.top < len(m.doc.Lines) {
		// a blank separator belongs to the block above; save the next text line instead
		i := m.top
		for i+1 < len(m.doc.Lines) && m.doc.Lines[i].Blank() {
			i++
		}
		l := m.doc.Lines[i]
		p.Chapter, p.Block = l.Chapter, l.Block
		p.Progress = m.percent()
	}
	return p
}

func (m *Model) persist() {
	if m.save != nil && m.doc != nil {
		m.save(m.position())
	}
}

func (m *Model) clamp() {
	m.top = max(0, min(m.top, len(m.doc.Lines)-m.bodyH()))
}

// current is the index of the TOC entry being read, or -1.
func (m *Model) current() int {
	cur := -1
	for i, it := range m.toc {
		if it.line >= 0 && it.line <= m.top {
			cur = i
		}
	}
	return cur
}

func (m *Model) jump(line int) {
	if line < 0 {
		return
	}
	m.top = line
	m.clamp()
	m.persist()
}

func (m *Model) nextChapter() {
	for _, it := range m.toc {
		if it.line > m.top {
			m.jump(it.line)
			return
		}
	}
}

func (m *Model) prevChapter() {
	cur := m.current()
	if cur >= 0 && m.toc[cur].line < m.top {
		m.jump(m.toc[cur].line)
		return
	}
	for i := cur - 1; i >= 0; i-- {
		if m.toc[i].line >= 0 && m.toc[i].line < m.top {
			m.jump(m.toc[i].line)
			return
		}
	}
	m.jump(0)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	if sync := m.syncImages(); sync != nil {
		cmd = tea.Batch(cmd, sync)
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		if m.overlay == overlayImage {
			return m, m.openImage(m.img.idx) // refit to the new size
		}
		return m, nil
	case noticeMsg:
		if msg.id == m.noticeID {
			m.notice = ""
		}
		return m, nil
	case tea.MouseMsg:
		if m.doc == nil {
			return m, nil
		}
		// any click clears the selection, wherever it lands; a press in the
		// text starts a new one below
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && !m.sel.dragging {
			m.sel.active = false
		}
		if m.overlay != overlayNone {
			return m, m.overlayMouse(msg)
		}
		return m, m.mouse(msg)
	case tea.KeyMsg:
		if m.doc == nil {
			return m, nil
		}
		if isMouseFragment(msg) {
			return m, nil
		}
		// Fast typing or key repeat delivers several runes in one message.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			for _, r := range msg.Runes {
				if cmd := m.key(string(r)); cmd != nil {
					return m, cmd
				}
			}
			return m, nil
		}
		return m, m.key(msg.String())
	}
	return m, nil
}

func (m *Model) key(k string) tea.Cmd {
	if k == "ctrl+c" {
		m.quitAll = true
		m.persist()
		return tea.Quit
	}
	m.sel.active = false
	if m.overlay != overlayNone {
		return m.overlayKey(k)
	}
	switch k {
	case "q":
		m.persist()
		return tea.Quit
	case "m":
		m.overlay, m.menuSel = overlayMenu, 0
		return nil
	case "?":
		m.overlay = overlayHelp
		return nil
	case "i":
		return m.openImage(m.firstVisibleImage())
	case "t":
		if m.sideBySide() {
			m.showTOC = !m.showTOC
			m.focusTOC = false
			m.layout()
		} else {
			m.focusTOC = !m.focusTOC
		}
		m.cursor = max(0, m.current())
		m.lastSel = -2
		return nil
	case "<", ">":
		d := 2
		if k == "<" {
			d = -2
		}
		m.resizeTOC(m.tocWidth() + d)
		m.persist()
		return nil
	case "tab":
		if m.sideBySide() && !m.showTOC {
			m.showTOC = true
			m.layout()
		}
		m.focusTOC = !m.focusTOC
		m.cursor = max(0, m.current())
		m.lastSel = -2
		return nil
	}
	if m.focusTOC {
		m.tocKey(k)
		return nil
	}

	page := max(1, m.bodyH()-2)
	switch k {
	case "j", "down":
		m.top++
	case "k", "up":
		m.top--
	case " ", "pgdown", "f":
		m.top += page
	case "b", "pgup":
		m.top -= page
	case "ctrl+d":
		m.top += page / 2
	case "ctrl+u":
		m.top -= page / 2
	case "g", "home":
		m.top = 0
	case "G", "end":
		m.top = len(m.doc.Lines)
	case "n":
		m.nextChapter()
	case "p":
		m.prevChapter()
	}
	m.clamp()
	return nil
}

// resizeTOC sets the sidebar width, keeping room for the reading area.
func (m *Model) resizeTOC(w int) {
	m.sideW = max(minTOC, min(w, maxTOC, m.w-minContent))
	m.layout()
}

func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	side := m.showTOC && m.sideBySide()
	overlay := m.focusTOC && !m.sideBySide()
	border := m.tocWidth() - 1
	inTOC := (side || overlay) && msg.X >= m.tocX && msg.X < m.tocX+m.tocW && msg.Y < m.bodyH()

	if m.sel.dragging {
		cmd, _ := m.selectMouse(msg)
		return cmd
	}
	switch {
	case m.dragging:
		switch msg.Action {
		case tea.MouseActionMotion:
			m.resizeTOC(msg.X + 1)
		case tea.MouseActionRelease:
			m.dragging = false
			m.persist()
		}
	case msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown:
		d := 3
		if msg.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		if inTOC {
			m.tocTop = max(0, m.tocTop+d) // clamped when drawn
		} else if !overlay {
			m.top += d
			m.clamp()
		}
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		switch {
		case m.onMenuButton(msg.X, msg.Y):
			m.overlay, m.menuSel = overlayMenu, 0
		case side && msg.X >= border-1 && msg.X <= border+1:
			m.dragging = true
		case inTOC:
			if idx := m.rowItem[msg.Y]; idx >= 0 {
				m.cursor = idx
				m.jump(m.toc[idx].line)
				m.focusTOC = false
			}
		case overlay:
			m.focusTOC = false
		case msg.Y < m.bodyH() && m.top+msg.Y < len(m.doc.Lines) && m.doc.Lines[m.top+msg.Y].Image > 0:
			return m.openImage(m.doc.Lines[m.top+msg.Y].Image - 1)
		default:
			cmd, _ := m.selectMouse(msg)
			return cmd
		}
	}
	return nil
}

func (m *Model) tocKey(k string) {
	switch k {
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.toc) - 1
	case "pgdown", " ", "ctrl+d":
		m.cursor += m.bodyH() / 2
	case "pgup", "b", "ctrl+u":
		m.cursor -= m.bodyH() / 2
	case "esc":
		m.focusTOC = false
	case "enter":
		if m.cursor < len(m.toc) {
			m.jump(m.toc[m.cursor].line)
		}
		m.focusTOC = false
	}
	m.cursor = max(0, min(m.cursor, len(m.toc)-1))
}

var styles = map[render.Style]lipgloss.Style{}

// headingColors are ANSI palette indexes, so the terminal's theme picks the
// actual colors. Normal (not bright) colors stay readable on light themes.
var headingColors = map[render.Style]lipgloss.Color{
	render.H1: "5", // magenta
	render.H2: "4", // blue
	render.H3: "6", // cyan

	render.CodeKeyword: "4", // blue
	render.CodeString:  "2", // green (also inline code)
	render.CodeNumber:  "6", // cyan
	render.CodeType:    "5", // magenta
}

// noColor honors https://no-color.org.
var noColor = os.Getenv("NO_COLOR") != ""

func styleFor(s render.Style) lipgloss.Style {
	st, ok := styles[s]
	if !ok {
		st = lipgloss.NewStyle().
			Bold(s&render.Bold != 0).
			Italic(s&render.Italic != 0).
			Faint(s&render.Faint != 0).
			Underline(s&render.Underline != 0)
		if !noColor {
			for level, c := range headingColors {
				if s&level != 0 {
					st = st.Foreground(c)
				}
			}
		}
		styles[s] = st
	}
	return st
}

var (
	faint   = lipgloss.NewStyle().Faint(true)
	bold    = lipgloss.NewStyle().Bold(true)
	reverse = lipgloss.NewStyle().Reverse(true)
)

func (m *Model) View() string {
	if m.doc == nil {
		return ""
	}
	bodyH := m.bodyH()
	rows := make([]string, 0, m.h)

	switch {
	case m.focusTOC && !m.sideBySide():
		width := min(m.w, 60)
		m.tocX, m.tocW = max(0, (m.w-width)/2), width
		margin := strings.Repeat(" ", m.tocX)
		for _, r := range m.sidebar(bodyH, width) {
			rows = append(rows, margin+r)
		}
	default:
		side := m.showTOC && m.sideBySide()
		var sb []string
		avail := m.w
		if side {
			m.tocX, m.tocW = 0, m.tocWidth()-2
			sb = m.sidebar(bodyH, m.tocW)
			avail -= m.tocWidth()
		}
		margin := strings.Repeat(" ", max(0, (avail-m.colW)/2))
		m.contentX = len(margin)
		if side {
			m.contentX += m.tocWidth()
		}
		for i := 0; i < bodyH; i++ {
			var b strings.Builder
			if side {
				b.WriteString(sb[i])
				b.WriteString(" ")
				b.WriteString(faint.Render("│"))
			}
			if li := m.top + i; li < len(m.doc.Lines) {
				b.WriteString(margin)
				if l := m.doc.Lines[li]; l.Image > 0 && l.ImageRow >= 0 && m.doc.Images[l.Image-1].Rows > 0 {
					im := m.doc.Images[l.Image-1]
					b.WriteString(strings.Repeat(" ", im.Pad))
					b.WriteString(kitty.PlaceholderRow(inlineID(l.Image-1), im.Cols, l.ImageRow))
				}
				c0, c1 := m.lineSel(li)
				b.WriteString(renderLine(m.doc.Lines[li].Spans, c0, c1))
			}
			rows = append(rows, b.String())
		}
	}
	rows = append(rows, m.status())
	if m.overlay != overlayNone {
		m.drawOverlay(rows)
	}
	return strings.Join(rows, "\n")
}

// entryRows word-wraps a TOC title into rows of at most width columns,
// continuation rows indented under the first word.
func (m *Model) entryRows(idx, width int) []string {
	it := m.toc[idx]
	indent := strings.Repeat("  ", min(it.depth, 4))
	mark := "  "
	if idx == m.current() {
		mark = "▸ "
	}
	first, rest := mark+indent, "  "+indent
	avail := max(4, width-runewidth.StringWidth(rest))

	var rows []string
	line, lineW := "", 0
	for _, word := range strings.Fields(it.title) {
		ww := runewidth.StringWidth(word)
		if lineW > 0 && lineW+1+ww > avail {
			rows = append(rows, line)
			line, lineW = "", 0
		}
		for ww > avail { // a single word wider than the sidebar
			head := runewidth.Truncate(word, avail, "")
			if line != "" {
				rows = append(rows, line)
			}
			rows = append(rows, head)
			word = word[len(head):]
			ww = runewidth.StringWidth(word)
			line, lineW = "", 0
		}
		if lineW > 0 {
			line += " "
			lineW++
		}
		line += word
		lineW += ww
	}
	if line != "" || len(rows) == 0 {
		rows = append(rows, line)
	}
	for i := range rows {
		p := rest
		if i == 0 {
			p = first
		}
		rows[i] = runewidth.FillRight(p+rows[i], width)
	}
	return rows
}

// sidebar renders exactly h rows of TOC, each padded to width columns,
// and records which entry each row belongs to for mouse clicks.
func (m *Model) sidebar(h, width int) []string {
	cur := m.current()
	sel := cur
	if m.focusTOC {
		sel = m.cursor
	}
	heights := make([]int, len(m.toc))
	for i := range m.toc {
		heights[i] = len(m.entryRows(i, width))
	}
	// lowest top that still fills the screen
	maxTop, used := len(m.toc), 0
	for maxTop > 0 && used+heights[maxTop-1] <= h {
		maxTop--
		used += heights[maxTop]
	}
	// scroll to the selection only when it moves, so the mouse wheel can browse
	if sel >= 0 && sel != m.lastSel {
		if sel < m.tocTop {
			m.tocTop = sel
		}
		for m.tocTop < sel {
			n := 0
			for i := m.tocTop; i <= sel; i++ {
				n += heights[i]
			}
			if n <= h {
				break
			}
			m.tocTop++
		}
	}
	m.lastSel = sel
	m.tocTop = max(0, min(m.tocTop, maxTop))

	rows := make([]string, 0, h)
	m.rowItem = m.rowItem[:0]
	for idx := m.tocTop; idx < len(m.toc) && len(rows) < h; idx++ {
		it := m.toc[idx]
		for _, text := range m.entryRows(idx, width) {
			if len(rows) == h {
				break
			}
			switch {
			case m.focusTOC && idx == m.cursor:
				text = reverse.Render(text)
			case idx == cur:
				text = bold.Render(text)
			case it.line < 0:
				text = faint.Render(text)
			}
			rows = append(rows, text)
			m.rowItem = append(m.rowItem, idx)
		}
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", width))
		m.rowItem = append(m.rowItem, -1)
	}
	return rows
}

func (m *Model) percent() int {
	if maxTop := len(m.doc.Lines) - m.bodyH(); maxTop > 0 {
		return m.top * 100 / maxTop
	}
	return 100
}

func (m *Model) status() string {
	title := m.book.Title
	if cur := m.current(); cur >= 0 {
		title = m.toc[cur].title
	}
	s := fmt.Sprintf("%s · %d%%", title, m.percent())
	if m.notice != "" {
		s = m.notice
	}
	btnW := runewidth.StringWidth(menuLabel)
	s = runewidth.Truncate(s, max(1, m.w-2*btnW-4), "…")
	sw := runewidth.StringWidth(s)
	pad := max(0, (m.w-sw)/2)
	btn := menuLabel
	if m.overlay == overlayMenu {
		btn = selStyle.Render(menuLabel)
	} else {
		btn = faint.Render(menuLabel)
	}
	gap := max(1, m.w-pad-sw-btnW)
	return strings.Repeat(" ", pad) + faint.Render(s) + strings.Repeat(" ", gap) + btn
}
