package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"

	"github.com/huyphung1602/tome/internal/library"
	"github.com/huyphung1602/tome/internal/state"
)

// Picker lists recently read books and every EPUB found under a root
// directory; typing filters the list. After it exits, Chosen holds the
// selected path ("" when the user quit).
type Picker struct {
	Chosen string

	lib    *library.Library
	recent []state.Position
	root   string
	home   string
	notice string // e.g. why the last book failed to open

	query   string
	all     []pickEntry
	shown   []pickEntry
	cursor  int
	offset  int // first visible list row
	scanned int
	done    bool
	w, h    int
	rowItem []int // list row -> index in shown, -1 for headers
}

type pickEntry struct {
	path, title, author string
	progress            int
	recent              bool
	key                 string // normalized text the filter matches against
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func NewPicker(lib *library.Library, recent []state.Position, root, home, notice string) *Picker {
	p := &Picker{lib: lib, recent: recent, root: root, home: home, notice: notice}
	p.refresh()
	return p
}

func (p *Picker) Init() tea.Cmd { return tick() }

// refresh rebuilds the entry list from recent positions and the scan.
func (p *Picker) refresh() {
	books, done := p.lib.Snapshot()
	if len(books) == p.scanned && done == p.done && p.all != nil {
		return
	}
	p.scanned, p.done = len(books), done
	seen := map[string]bool{}
	p.all = p.all[:0]
	for _, r := range p.recent {
		seen[r.Path] = true
		p.all = append(p.all, newEntry(r.Path, r.Title, r.Author, r.Progress, true))
	}
	var found []pickEntry
	for _, b := range books {
		if !seen[b.Path] {
			found = append(found, newEntry(b.Path, b.Title, b.Author, -1, false))
		}
	}
	slices.SortStableFunc(found, func(a, b pickEntry) int { return strings.Compare(fold(a.title), fold(b.title)) })
	p.all = append(p.all, found...)
	p.filter()
}

func newEntry(path, title, author string, progress int, recent bool) pickEntry {
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return pickEntry{
		path: path, title: title, author: author, progress: progress, recent: recent,
		key: fold(title + " " + author + " " + filepath.Base(path)),
	}
}

// fold lowercases and strips diacritics so "dam bi ghet" matches "Dám Bị Ghét".
func fold(s string) string {
	var sb strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'đ':
			sb.WriteRune('d')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (p *Picker) filter() {
	var cur string
	if p.cursor < len(p.shown) {
		cur = p.shown[p.cursor].path
	}
	words := strings.Fields(fold(p.query))
	p.shown = p.shown[:0]
	for _, e := range p.all {
		ok := true
		for _, w := range words {
			if !strings.Contains(e.key, w) {
				ok = false
				break
			}
		}
		if ok {
			p.shown = append(p.shown, e)
		}
	}
	// keep the cursor on the same book as results stream in
	p.cursor = 0
	for i, e := range p.shown {
		if e.path == cur {
			p.cursor = i
		}
	}
}

func (p *Picker) listH() int { return max(1, p.h-5) }

func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		p.refresh()
		if p.done {
			return p, nil
		}
		return p, tick()
	case tea.WindowSizeMsg:
		p.w, p.h = msg.Width, msg.Height
	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			p.move(-3)
		case msg.Button == tea.MouseButtonWheelDown:
			p.move(3)
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
			if row := msg.Y - 3 + p.offset; msg.Y >= 3 && row < len(p.rowItem) && p.rowItem[row] >= 0 {
				p.cursor = p.rowItem[row]
				return p, p.open()
			}
		}
	case tea.KeyMsg:
		if isMouseFragment(msg) {
			return p, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			return p, tea.Quit
		case tea.KeyEsc:
			if p.query == "" {
				return p, tea.Quit
			}
			p.query = ""
			p.filter()
		case tea.KeyEnter:
			return p, p.open()
		case tea.KeyUp, tea.KeyCtrlP:
			p.move(-1)
		case tea.KeyDown, tea.KeyCtrlN:
			p.move(1)
		case tea.KeyPgUp:
			p.move(-p.listH())
		case tea.KeyPgDown:
			p.move(p.listH())
		case tea.KeyBackspace:
			if r := []rune(p.query); len(r) > 0 {
				p.query = string(r[:len(r)-1])
				p.filter()
			}
		case tea.KeyCtrlU:
			p.query = ""
			p.filter()
		case tea.KeyRunes, tea.KeySpace:
			p.query += string(msg.Runes)
			p.filter()
		}
	}
	return p, nil
}

func (p *Picker) move(d int) {
	p.cursor = max(0, min(p.cursor+d, len(p.shown)-1))
}

func (p *Picker) open() tea.Cmd {
	if p.cursor < len(p.shown) {
		p.Chosen = p.shown[p.cursor].path
		return tea.Quit
	}
	return nil
}

func (p *Picker) View() string {
	if p.w == 0 {
		return ""
	}
	width := min(p.w-4, 110)
	margin := strings.Repeat(" ", max(0, (p.w-width)/2))
	rows := []string{
		margin + bold.Render("tome") + faint.Render("  pick a book"),
		margin + "> " + p.query + reverse.Render(" "),
		"",
	}

	// list rows: section headers + entries
	var list []string
	p.rowItem = p.rowItem[:0]
	header := func(s string) {
		list = append(list, faint.Render(bold.Render(s)))
		p.rowItem = append(p.rowItem, -1)
	}
	lastRecent := false
	for i, e := range p.shown {
		if p.query == "" {
			if i == 0 && e.recent {
				header("RECENT")
			}
			if !e.recent && (i == 0 || lastRecent) {
				if i > 0 {
					list = append(list, "")
					p.rowItem = append(p.rowItem, -1)
				}
				header("ALL BOOKS")
			}
		}
		lastRecent = e.recent
		list = append(list, p.entryRow(e, i == p.cursor, width))
		p.rowItem = append(p.rowItem, i)
	}
	if len(p.shown) == 0 {
		msg := "no matches"
		switch {
		case p.query == "" && p.done:
			msg = "no EPUB files found under " + p.tilde(p.root)
		case p.query == "":
			msg = "looking for books…"
		}
		list = append(list, faint.Render(msg))
	}

	// keep the cursor row visible
	h := p.listH()
	curRow := 0
	for r, idx := range p.rowItem {
		if idx == p.cursor {
			curRow = r
			// show the section header above the first entry
			if r > 0 && p.rowItem[r-1] == -1 && r-1 < p.offset {
				p.offset = r - 1
			}
		}
	}
	if curRow < p.offset {
		p.offset = curRow
	} else if curRow >= p.offset+h {
		p.offset = curRow - h + 1
	}
	p.offset = max(0, min(p.offset, len(list)-h))
	for r := p.offset; r < min(len(list), p.offset+h); r++ {
		rows = append(rows, margin+list[r])
	}
	for len(rows) < p.h-2 {
		rows = append(rows, "")
	}

	status := fmt.Sprintf("%d books", len(p.all))
	if !p.done {
		status += " · scanning " + p.tilde(p.root) + "…"
	}
	if p.notice != "" {
		status = p.notice
	}
	hint := "↑↓ select · enter open · esc quit"
	gap := max(2, width-runewidth.StringWidth(hint)-runewidth.StringWidth(status))
	rows = append(rows, "", margin+faint.Render(hint+strings.Repeat(" ", gap)+status))
	return strings.Join(rows[:min(len(rows), p.h)], "\n")
}

// entryRow renders "▸ Title — Author        34%  ~/dir" within width.
func (p *Picker) entryRow(e pickEntry, sel bool, width int) string {
	mark := "  "
	if sel {
		mark = "▸ "
	}
	right := "    "
	if e.progress >= 0 {
		right = fmt.Sprintf("%3d%%", e.progress)
	}
	dir := p.tilde(filepath.Dir(e.path))
	dirW := min(32, max(10, width/3))
	right += "  " + runewidth.FillRight(runewidth.Truncate(dir, dirW, "…"), dirW)

	leftW := max(10, width-runewidth.StringWidth(mark)-runewidth.StringWidth(right)-2)
	title := runewidth.Truncate(e.title, leftW, "…")
	author := ""
	if e.author != "" && runewidth.StringWidth(title)+3 < leftW {
		author = runewidth.Truncate(" — "+e.author, leftW-runewidth.StringWidth(title), "…")
	}
	pad := strings.Repeat(" ", max(0, leftW-runewidth.StringWidth(title)-runewidth.StringWidth(author)))

	t := title
	if sel {
		t = bold.Render(title)
	}
	return mark + t + faint.Render(author) + pad + "  " + faint.Render(right)
}

func (p *Picker) tilde(path string) string {
	if p.home != "" && (path == p.home || strings.HasPrefix(path, p.home+string(filepath.Separator))) {
		return "~" + path[len(p.home):]
	}
	return path
}
