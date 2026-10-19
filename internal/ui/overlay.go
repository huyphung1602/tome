package ui

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/kitty"
	"github.com/huyphung1602/tome/internal/render"
)

// Overlays follow herdr's model: the reading view is drawn first, then a
// bordered box is spliced over it; one overlay is open at a time.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayMenu
	overlayHelp
	overlayImage
	overlayBookmarks
)

const (
	menuLabel = "● menu"
	keysLabel = "? keybinds"
	tocLabel  = "t contents"
)

type menuAction int

const (
	menuHelp menuAction = iota
	menuTOC
	menuImages
	menuSearch
	menuBookmarks
	menuBack
	menuQuit
)

type menuEntry struct {
	label, key string
	action     menuAction
}

// menuEntries lists what the menu offers right now, each with its shortcut
// so the menu also teaches the keys.
func (m *Model) menuEntries() []menuEntry {
	toc := "show contents"
	if m.showTOC && m.sideBySide() {
		toc = "hide contents"
	}
	items := []menuEntry{
		{"keybinds", "?", menuHelp},
		{toc, "t", menuTOC},
		{"search", "/", menuSearch},
		{"bookmarks", "'", menuBookmarks},
	}
	if m.doc != nil && len(m.doc.Images) > 0 {
		items = append(items, menuEntry{"images", "i", menuImages})
	}
	if m.bookList {
		items = append(items, menuEntry{"back to book list", "q", menuBack}, menuEntry{"quit", "ctrl+c", menuQuit})
	} else {
		items = append(items, menuEntry{"quit", "q", menuQuit})
	}
	return items
}

// helpRows are the keybinding sheet; mouse and copy rows explain what the
// terminal's usual gestures do inside tome.
func (m *Model) helpRows() [][2]string {
	quit := [][2]string{{"q", "save position & quit"}}
	if m.bookList {
		quit = [][2]string{{"q", "save position, back to book list"}, {"ctrl+c", "quit tome"}}
	}
	rows := [][2]string{
		{"j k  ↓ ↑", "move the cursor (dim ◆); the page scrolls at the edges"},
		{"space b  l h  → ←", "page down / up"},
		{"ctrl+d ctrl+u", "half page down / up (cursor comes along)"},
		{"g G", "start / end of book"},
		{"n p", "next / previous chapter"},
		{"", ""},
		{"/", "search (no accents typed = accents ignored)"},
		{"n N", "next / previous match (esc clears)"},
		{"{ }", "previous / next paragraph"},
		{"B", "bookmark / unbookmark the cursor's paragraph"},
		{"'", "list bookmarks"},
		{"", ""},
		{"t", "show / hide contents"},
		{"tab", "focus contents (enter jumps, esc leaves)"},
		{"< >", "narrower / wider contents"},
		{"- +", "shorter / taller reading frame"},
		{"i", "view the image on screen (← → next, o open)"},
		{"m  ?", "menu, this help"},
		{"", ""},
		{"drag", "select text, copied when you let go"},
		{"double-click", "select and copy a paragraph"},
		{"shift+drag", "the terminal's own selection"},
		{"click", "move the cursor; contents entry, image, left margin (bookmark)"},
		{"drag │", "resize the contents"},
		{"", ""},
	}
	return append(rows, quit...)
}

// imageView is the state of the image modal.
type imageView struct {
	idx        int // index into doc.Images
	id         uint32
	cols, rows int
	data       []byte // original bytes, for "open externally"
	err        string
}

var (
	boxStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	selStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
)

// EnableImages turns on kitty graphics output, written to w. With inline,
// images are drawn in the reading column; otherwise only in the modal.
// cellW/cellH is the terminal cell size in pixels (0 = guess).
func (m *Model) EnableImages(w io.Writer, inline bool, cellW, cellH int) {
	m.gfx, m.inline = w, inline
	m.cellW, m.cellH = cellW, cellH
	if m.cellW <= 0 || m.cellH <= 0 {
		m.cellW, m.cellH = kitty.CellSize()
	}
}

// inlineID is the kitty image id for inline image idx; modal images use 1 and 2.
func inlineID(idx int) uint32 { return uint32(idx) + 100 }

// pixelSize returns an image's pixel size, cached per source path.
func (m *Model) pixelSize(src string) (w, h int, ok bool) {
	if m.imgSize == nil {
		m.imgSize = map[string][2]int{}
	}
	if s, hit := m.imgSize[src]; hit {
		return s[0], s[1], s[0] > 0
	}
	if data, err := m.book.ReadFile(src); err == nil {
		w, h, _ = kitty.Size(data)
	}
	m.imgSize[src] = [2]int{w, h}
	return w, h, w > 0
}

// fitInline sizes an inline image: at most the text column wide and the
// reading area tall, never larger than its natural size.
func (m *Model) fitInline(src string, maxCols int) (cols, rows int) {
	w, h, ok := m.pixelSize(src)
	if !ok {
		return 0, 0
	}
	return kitty.Fit(w, h, maxCols, max(1, m.bodyH()-2), m.cellW, m.cellH)
}

// syncImages transmits inline images on or near the screen that the
// terminal doesn't have at their current size yet.
func (m *Model) syncImages() tea.Cmd {
	if !m.inline || m.gfx == nil || m.doc == nil {
		return nil
	}
	if m.sent == nil {
		m.sent = map[int][2]int{}
	}
	var cmds []tea.Cmd
	from, to := max(0, m.top-m.bodyH()), min(len(m.doc.Lines), m.top+2*m.bodyH())
	for i := from; i < to; i++ {
		l := m.doc.Lines[i]
		if l.Image == 0 || l.ImageRow != 0 {
			continue
		}
		idx := l.Image - 1
		im := m.doc.Images[idx]
		size := [2]int{im.Cols, im.Rows}
		if im.Rows == 0 || m.sent[idx] == size {
			continue
		}
		m.sent[idx] = size
		book, w, id := m.book, m.gfx, inlineID(idx)
		cmds = append(cmds, func() tea.Msg {
			data, err := book.ReadFile(im.Src)
			if err != nil {
				return nil
			}
			pngData, _, _, err := kitty.Decode(data)
			if err != nil {
				return nil
			}
			out := append(kitty.Delete(id), kitty.Transmit(id, pngData, size[0], size[1])...)
			w.Write(out)
			return nil
		})
	}
	return tea.Batch(cmds...)
}

// CleanupGraphics returns the escapes that free every image this session
// uploaded; write it after the UI exits.
func (m *Model) CleanupGraphics() []byte {
	if m.gfx == nil {
		return nil
	}
	var out []byte
	for idx := range m.sent {
		out = append(out, kitty.Delete(inlineID(idx))...)
	}
	for _, id := range []uint32{1, 2} {
		out = append(out, kitty.Delete(id)...)
	}
	return out
}

func (m *Model) writeCmd(data []byte) tea.Cmd {
	if m.gfx == nil || len(data) == 0 {
		return nil
	}
	w := m.gfx
	return func() tea.Msg {
		w.Write(data)
		return nil
	}
}

func (m *Model) closeOverlay() tea.Cmd {
	var cmd tea.Cmd
	if m.overlay == overlayImage && m.img.id != 0 {
		cmd = m.writeCmd(kitty.Delete(m.img.id))
	}
	m.overlay = overlayNone
	m.img = imageView{}
	return cmd
}

// openImage shows doc.Images[idx] in the image modal.
func (m *Model) openImage(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.doc.Images) {
		return nil
	}
	prev := m.img.id
	m.overlay = overlayImage
	m.img = imageView{idx: idx}
	im := m.doc.Images[idx]
	if im.Src == "" {
		m.img.err = "the book doesn't say which file this is"
		return m.deleteCmd(prev)
	}
	data, err := m.book.ReadFile(im.Src)
	if err != nil {
		m.img.err = "can't read " + filepath.Base(im.Src)
		return m.deleteCmd(prev)
	}
	m.img.data = data
	if m.gfx == nil {
		m.img.err = "this terminal can't display images"
		return m.deleteCmd(prev)
	}
	pngData, w, h, err := kitty.Decode(data)
	if err != nil {
		m.img.err = "unsupported image format"
		return m.deleteCmd(prev)
	}
	maxCols, maxRows := m.imageArea()
	m.img.cols, m.img.rows = kitty.Fit(w, h, maxCols, maxRows, m.cellW, m.cellH)
	// alternate ids so the new image never shows the old one's pixels
	m.img.id = 1
	if prev == 1 {
		m.img.id = 2
	}
	return tea.Batch(m.deleteCmd(prev), m.writeCmd(kitty.Transmit(m.img.id, pngData, m.img.cols, m.img.rows)))
}

func (m *Model) deleteCmd(id uint32) tea.Cmd {
	if id == 0 {
		return nil
	}
	return m.writeCmd(kitty.Delete(id))
}

// imageArea is the space for the picture inside the modal, leaving room for
// border, caption and footer.
func (m *Model) imageArea() (cols, rows int) {
	captionRows := 0
	if c := m.caption(); c != "" {
		captionRows = len(wrapPlain(c, max(20, m.w-8))) + 1
	}
	return max(1, m.w-8), max(1, m.h-4-captionRows-2)
}

func (m *Model) caption() string {
	if m.overlay != overlayImage || m.img.idx >= len(m.doc.Images) {
		return ""
	}
	im := m.doc.Images[m.img.idx]
	if im.Caption != "" {
		return im.Caption
	}
	return im.Alt
}

func (m *Model) overlayKey(k string) tea.Cmd {
	switch m.overlay {
	case overlayMenu:
		switch k {
		case "j", "down", "tab":
			m.menuSel = min(m.menuSel+1, len(m.menuEntries())-1)
		case "k", "up", "shift+tab":
			m.menuSel = max(m.menuSel-1, 0)
		case "enter", " ":
			return m.activateMenu(m.menuSel)
		case "esc", "m", "q":
			return m.closeOverlay()
		}
	case overlayHelp:
		switch k {
		case "esc", "?", "q", "enter":
			return m.closeOverlay()
		}
	case overlayBookmarks:
		m.bookmarksKey(k)
	case overlayImage:
		switch k {
		case "esc", "q", "i", "enter":
			return m.closeOverlay()
		case "left", "h", "k", "up":
			if m.img.idx > 0 {
				return m.openImage(m.img.idx - 1)
			}
		case "right", "l", "j", "down":
			if m.img.idx < len(m.doc.Images)-1 {
				return m.openImage(m.img.idx + 1)
			}
		case "o":
			m.openExternal()
		}
	}
	return nil
}

func (m *Model) activateMenu(i int) tea.Cmd {
	items := m.menuEntries()
	if i < 0 || i >= len(items) {
		return nil
	}
	m.overlay = overlayNone
	switch items[i].action {
	case menuHelp:
		m.overlay = overlayHelp
	case menuTOC:
		return m.key("t")
	case menuSearch:
		m.openSearch()
	case menuBookmarks:
		m.overlay, m.bmSel = overlayBookmarks, 0
	case menuImages:
		idx := m.firstVisibleImage()
		if idx < 0 {
			idx = m.nextImageAfter(m.top)
		}
		return m.openImage(idx)
	case menuBack:
		m.persist()
		return tea.Quit
	case menuQuit:
		if m.bookList {
			m.quitAll = true
		}
		m.persist()
		return tea.Quit
	}
	return nil
}

// nextImageAfter is the first image at or after line, else the last image.
func (m *Model) nextImageAfter(line int) int {
	for i, im := range m.doc.Images {
		if im.Line >= line {
			return i
		}
	}
	return len(m.doc.Images) - 1
}

// openExternal writes the image to a temp file and opens the system viewer.
func (m *Model) openExternal() {
	if len(m.img.data) == 0 {
		return
	}
	dir := filepath.Join(os.TempDir(), "tome-images")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	p := filepath.Join(dir, filepath.Base(m.doc.Images[m.img.idx].Src))
	if os.WriteFile(p, m.img.data, 0o644) != nil {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	cmd.Start()
}

// firstVisibleImage is the first image placeholder on screen, or -1.
func (m *Model) firstVisibleImage() int {
	for i := m.top; i < min(m.top+m.bodyH(), len(m.doc.Lines)); i++ {
		if img := m.doc.Lines[i].Image; img > 0 {
			return img - 1
		}
	}
	return -1
}

// box renders lines inside a bordered box; sel (or -1) is drawn reversed
// across the full inner width.
func box(lines []string, innerW, sel int) []string {
	rows := make([]string, len(lines))
	for i, l := range lines {
		l = ansi.Truncate(l, innerW, "…")
		pad := strings.Repeat(" ", max(0, innerW-ansi.StringWidth(l)))
		if i == sel {
			rows[i] = selStyle.Render(ansi.Strip(l) + pad)
		} else {
			rows[i] = l + pad
		}
	}
	return strings.Split(boxStyle.Render(strings.Join(rows, "\n")), "\n")
}

// place splices box rows over base rows at column x, row y.
func place(base []string, rows []string, x, y int) {
	for i, r := range rows {
		by := y + i
		if by < 0 || by >= len(base) {
			continue
		}
		w := ansi.StringWidth(r)
		left := ansi.Truncate(base[by], x, "")
		if gap := x - ansi.StringWidth(left); gap > 0 {
			left += strings.Repeat(" ", gap)
		}
		right := ansi.TruncateLeft(base[by], x+w, "")
		base[by] = left + "\x1b[0m" + r + "\x1b[0m" + right
	}
}

// overlayRows draws the open overlay over the full-screen rows and records
// hit areas for the mouse.
func (m *Model) drawOverlay(rows []string) {
	switch m.overlay {
	case overlayMenu:
		items := m.menuEntries()
		labelW, keyW := 0, 0
		for _, it := range items {
			labelW = max(labelW, runewidth.StringWidth(it.label))
			keyW = max(keyW, runewidth.StringWidth(it.key))
		}
		innerW := labelW + 4 + keyW
		lines := make([]string, len(items))
		for i, it := range items {
			gap := strings.Repeat(" ", innerW-runewidth.StringWidth(it.label)-runewidth.StringWidth(it.key))
			if i == m.menuSel {
				lines[i] = it.label + gap + it.key // box() reverses the whole row
			} else {
				lines[i] = it.label + gap + faint.Render(it.key)
			}
		}
		b := box(lines, innerW, m.menuSel)
		w := ansi.StringWidth(b[0])
		// right edge lined up with the menu button
		x := max(0, min(m.w-w, m.menuX+runewidth.StringWidth(menuLabel)-w))
		y := m.barY() + 1 // drops down from the button
		place(rows, b, x, y)
		m.hit = rect{x, y + 1, w, len(items)}
	case overlayHelp:
		helpRows := m.helpRows()
		keyW := 0
		for _, r := range helpRows {
			keyW = max(keyW, runewidth.StringWidth(r[0]))
		}
		lines := []string{bold.Render("keybinds"), ""}
		for _, r := range helpRows {
			if r[0] == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, bold.Render(runewidth.FillRight(r[0], keyW))+"   "+r[1])
		}
		// too tall for the screen: put the rows in two columns
		if body := lines[2:]; len(lines)+4 > m.h && len(body) > 1 {
			// break at the group boundary (blank row) closest to the middle
			half := (len(body) + 1) / 2
			best := -1
			for i, l := range body {
				if l == "" && (best < 0 || abs(i-half) < abs(best-half)) {
					best = i
				}
			}
			if best > 0 {
				half = best
			}
			left, right := body[:half], body[min(half+1, len(body)):]
			colW := 0
			for _, l := range left {
				colW = max(colW, ansi.StringWidth(l))
			}
			merged := lines[:2:2]
			for i := 0; i < max(len(left), len(right)); i++ {
				l, r := "", ""
				if i < len(left) {
					l = left[i]
				}
				if i < len(right) {
					r = right[i]
				}
				merged = append(merged, l+strings.Repeat(" ", colW-ansi.StringWidth(l)+4)+r)
			}
			lines = merged
		}
		lines = append(lines, "", faint.Render("esc close"))
		innerW := 0
		for _, l := range lines {
			innerW = max(innerW, ansi.StringWidth(l))
		}
		innerW = min(innerW, m.w-4)
		b := box(lines, innerW, -1)
		m.hit = m.center(rows, b)
	case overlayBookmarks:
		innerW := min(m.w-6, 90)
		lines := m.bookmarkRows(innerW)
		sel := -1
		if len(m.bookmarks) > 0 {
			sel = 2 + m.bmSel
		}
		b := box(lines, innerW, sel)
		m.hit = m.center(rows, b)
	case overlayImage:
		var lines []string
		if m.img.err == "" {
			lines = append(lines, kitty.Placeholder(m.img.id, m.img.cols, m.img.rows)...)
		} else {
			lines = append(lines, faint.Render("[Image] "+m.img.err))
		}
		innerW := max(m.img.cols, 40)
		if c := m.caption(); c != "" {
			lines = append(lines, "")
			for _, l := range wrapPlain(c, innerW) {
				lines = append(lines, styleFor(render.Italic|render.Faint).Render(l))
			}
		}
		hint := "esc close"
		if len(m.img.data) > 0 {
			hint = "o open in viewer · " + hint
		}
		if len(m.doc.Images) > 1 {
			hint = "← → prev/next · " + hint
		}
		lines = append(lines, "", faint.Render(hint+"  ("+strconv.Itoa(m.img.idx+1)+"/"+strconv.Itoa(len(m.doc.Images))+")"))
		for _, l := range lines {
			innerW = max(innerW, ansi.StringWidth(l))
		}
		innerW = min(innerW, m.w-4)
		b := box(lines, innerW, -1)
		m.hit = m.center(rows, b)
	}
}

func (m *Model) center(rows, b []string) rect {
	w := ansi.StringWidth(b[0])
	x := max(0, (m.w-w)/2)
	y := max(0, (m.h-len(b))/2)
	place(rows, b, x, y)
	return rect{x, y, w, len(b)}
}

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (m *Model) overlayMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if m.overlay == overlayMenu {
		if m.hit.contains(msg.X, msg.Y) {
			return m.activateMenu(msg.Y - m.hit.y)
		}
		if m.onMenuButton(msg.X, msg.Y) {
			m.overlay = overlayNone
			return nil
		}
	}
	if !m.hit.contains(msg.X, msg.Y) {
		return m.closeOverlay()
	}
	if m.overlay == overlayBookmarks {
		// rows: border, title, blank, then one per bookmark
		if i := msg.Y - m.hit.y - 3; i >= 0 && i < len(m.bookmarks) {
			m.overlay = overlayNone
			m.jumpToBookmark(i)
		}
	}
	return nil
}

func (m *Model) onMenuButton(x, y int) bool {
	return y == m.barY() && x >= m.menuX && x < m.menuX+runewidth.StringWidth(menuLabel)
}

func (m *Model) onTOCButton(x, y int) bool {
	return m.tocBtnX >= 0 && y == m.barY() && x >= m.tocBtnX && x < m.tocBtnX+runewidth.StringWidth(tocLabel)
}

func (m *Model) onKeysButton(x, y int) bool {
	return m.keysX >= 0 && y == m.barY() && x >= m.keysX && x < m.keysX+runewidth.StringWidth(keysLabel)
}

func wrapPlain(s string, width int) []string {
	var rows []string
	line, lw := "", 0
	for _, word := range strings.Fields(s) {
		ww := runewidth.StringWidth(word)
		if lw > 0 && lw+1+ww > width {
			rows = append(rows, line)
			line, lw = "", 0
		}
		if lw > 0 {
			line += " "
			lw++
		}
		line += word
		lw += ww
	}
	if line != "" {
		rows = append(rows, line)
	}
	return rows
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
