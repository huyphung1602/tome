package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/state"
)

func selBook() *epub.Book {
	html := `<h1>Title</h1>
<p>The quick brown fox jumps over the lazy dog and keeps running far away.</p>
<ul><li>first item that is long enough to wrap onto a second line here</li><li>second</li></ul>
<blockquote><p>quoted text</p></blockquote>
<pre>  x = 1
  y = 2</pre>
<p>last</p>`
	return &epub.Book{Title: "T", Chapters: []epub.Chapter{{Path: "c.html", HTML: []byte(html)}}}
}

func selModel(t *testing.T) (*Model, *string) {
	t.Helper()
	var copied string
	m := New(selBook(), state.Position{HideTOC: true}, nil)
	m.SetClipboard(func(s string) error { copied = s; return nil })
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 30}) // narrow: column 36, no sidebar
	m.View()
	return m, &copied
}

// screen returns the screen position of the first occurrence of text.
func screen(t *testing.T, m *Model, text string) (x, y int) {
	t.Helper()
	for i := m.top; i < len(m.doc.Lines); i++ {
		if c := strings.Index(m.doc.Lines[i].Plain(), text); c >= 0 {
			return m.contentX + len([]rune(m.doc.Lines[i].Plain()[:c])), m.textTop() + i - m.top
		}
	}
	t.Fatalf("%q not on screen", text)
	return
}

func drag(m *Model, x0, y0, x1, y1 int) tea.Cmd {
	m.Update(mouse(x0, y0, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(x1, y1, tea.MouseButtonLeft, tea.MouseActionMotion))
	_, cmd := m.Update(mouse(x1, y1, tea.MouseButtonLeft, tea.MouseActionRelease))
	return cmd
}

func TestDragCopiesCleanText(t *testing.T) {
	m, copied := selModel(t)
	x0, y0 := screen(t, m, "Title")
	x1, y1 := screen(t, m, "last")
	drag(m, x0, y0, x1+4, y1)

	want := "Title\n\nThe quick brown fox jumps over the lazy dog and keeps running far away.\n\n" +
		"• first item that is long enough to wrap onto a second line here\n• second\n\n" +
		"quoted text\n\n  x = 1\n  y = 2\n\nlast"
	if *copied != want {
		t.Errorf("copied\n%q\nwant\n%q", *copied, want)
	}
	if !strings.Contains(m.status(), "copied") {
		t.Errorf("status = %q", m.status())
	}
	if c0, c1 := m.lineSel(y0 - m.textTop() + m.top); c0 >= c1 {
		t.Error("selection not highlighted")
	}
	if got := renderLine(m.doc.Lines[y1-m.textTop()+m.top].Spans, 0, 2, nil, nil); !strings.Contains(got, "la") {
		t.Errorf("renderLine dropped text: %q", got)
	}
}

func TestDragWithinLine(t *testing.T) {
	m, copied := selModel(t)
	x, y := screen(t, m, "quick")
	drag(m, x, y, x+len("quick brown"), y)
	if *copied != "quick brown" {
		t.Errorf("copied %q", *copied)
	}
	// dragging backwards gives the same text
	*copied = ""
	drag(m, x+len("quick brown"), y, x, y)
	if *copied != "quick brown" {
		t.Errorf("backwards copied %q", *copied)
	}
}

func TestClickWithoutDragDoesNotCopy(t *testing.T) {
	m, copied := selModel(t)
	x, y := screen(t, m, "quick")
	drag(m, x, y, x, y)
	if *copied != "" || m.sel.active {
		t.Errorf("plain click copied %q", *copied)
	}
}

func TestDoubleClickSelectsParagraph(t *testing.T) {
	m, copied := selModel(t)
	x, y := screen(t, m, "lazy")
	m.Update(mouse(x, y, tea.MouseButtonLeft, tea.MouseActionPress))
	m.Update(mouse(x, y, tea.MouseButtonLeft, tea.MouseActionRelease))
	m.Update(mouse(x, y, tea.MouseButtonLeft, tea.MouseActionPress))
	if *copied != "The quick brown fox jumps over the lazy dog and keeps running far away." {
		t.Errorf("double-click copied %q", *copied)
	}
}

func TestKeyClearsSelection(t *testing.T) {
	m, _ := selModel(t)
	x, y := screen(t, m, "quick")
	drag(m, x, y, x+5, y)
	keys(m, runes("j"))
	if m.sel.active {
		t.Error("selection survived a key press")
	}
}

func TestMenuEntriesDependOnContext(t *testing.T) {
	m := New(imageBook(t), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	labels := func() string {
		var l []string
		for _, e := range m.menuEntries() {
			l = append(l, e.label+"="+e.key)
		}
		return strings.Join(l, ",")
	}
	if got := labels(); got != "keybinds=?,hide contents=t,search=/,bookmarks=',images=i,quit=q" {
		t.Errorf("standalone menu = %s", got)
	}
	m.SetBookList(true)
	if got := labels(); !strings.HasSuffix(got, "back to book list=q,quit=ctrl+c") {
		t.Errorf("book-list menu = %s", got)
	}
	keys(m, runes("m"))
	if v := m.View(); !strings.Contains(v, "back to book list") || !strings.Contains(v, "ctrl+c") {
		t.Errorf("menu view:\n%s", v)
	}
	// quit from the menu leaves the whole app when opened from the list
	keys(m, runes("jjjjjjjj"), tea.KeyMsg{Type: tea.KeyEnter})
	if !m.QuitAll() {
		t.Error("menu quit should quit the app")
	}
}

func TestClickAnywhereClearsSelection(t *testing.T) {
	var copied string
	m := New(selBook(), state.Position{}, nil) // sidebar shown
	m.SetClipboard(func(s string) error { copied = s; return nil })
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m.View()
	x, y := screen(t, m, "quick")
	for name, at := range map[string][2]int{
		"sidebar":    {2, 0},
		"margin":     {m.w - 3, 5},
		"status bar": {10, m.h - 1},
		"menu":       {m.w - 1, m.h - 1},
	} {
		drag(m, x, y, x+5, y)
		if !m.sel.active || copied == "" {
			t.Fatalf("%s: no selection to clear", name)
		}
		m.Update(mouse(at[0], at[1], tea.MouseButtonLeft, tea.MouseActionPress))
		m.Update(mouse(at[0], at[1], tea.MouseButtonLeft, tea.MouseActionRelease))
		if m.sel.active {
			t.Errorf("click on %s kept the selection", name)
		}
		m.overlay = overlayNone
	}
	// the wheel keeps it
	drag(m, x, y, x+5, y)
	m.Update(mouse(x, y, tea.MouseButtonWheelDown, tea.MouseActionPress))
	if !m.sel.active {
		t.Error("scrolling cleared the selection")
	}
}

func TestCopyWrappedCodeRejoinsLines(t *testing.T) {
	long := strings.Repeat("x", 50)
	b := &epub.Book{Chapters: []epub.Chapter{{Path: "c.html", HTML: []byte(
		"<pre>  short\n  " + long + "\nend</pre>")}}}
	var copied string
	m := New(b, state.Position{HideTOC: true}, nil)
	m.SetClipboard(func(s string) error { copied = s; return nil })
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	m.View()
	_, y0 := screen(t, m, "short")
	_, y1 := screen(t, m, "end")
	drag(m, m.contentX-2, y0-1, m.w-1, y1+1)
	if want := "  short\n  " + long + "\nend"; copied != want {
		t.Errorf("copied %q\nwant   %q", copied, want)
	}
}
