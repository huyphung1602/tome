package ui

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/state"
)

func run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b {
			run(c)
		}
	}
}

func imageBook(t *testing.T) *epub.Book {
	t.Helper()
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 160, 80)))
	files := map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf": `<package><manifest><item id="c" href="c.html" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="c"/></spine></package>`,
		"OEBPS/c.html": `<p>before</p><figure><img src="img/a.png" alt="alt text"/><figcaption>Hình 1. Một bức tranh</figcaption></figure>
<p>between</p><img src="img/a.png"/>`,
		"OEBPS/img/a.png": pngBuf.String(),
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(b))
	}
	zw.Close()
	p := filepath.Join(t.TempDir(), "img.epub")
	os.WriteFile(p, buf.Bytes(), 0o644)
	b, err := epub.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMenuOpensHelp(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if !strings.Contains(m.View(), "● menu") {
		t.Fatal("status bar has no menu button")
	}
	keys(m, runes("m"))
	v := m.View()
	if m.overlay != overlayMenu || !strings.Contains(v, "keybindings") || !strings.Contains(v, "quit") {
		t.Fatalf("menu not shown:\n%s", v)
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != overlayHelp || !strings.Contains(m.View(), "esc close") {
		t.Fatal("help not shown")
	}
	keys(m, runes("j"))
	if m.top != 0 {
		t.Error("keys leaked to the reader while help is open")
	}
	keys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.overlay != overlayNone {
		t.Error("esc did not close help")
	}
}

func TestMenuByMouse(t *testing.T) {
	m := New(testBook(), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.View()
	m.Update(mouse(119, 23, tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayMenu {
		t.Fatal("click on menu button did not open menu")
	}
	m.View()
	m.Update(mouse(m.hit.x+2, m.hit.y, tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayHelp {
		t.Fatalf("click on first item gave overlay %d", m.overlay)
	}
	m.View()
	m.Update(mouse(0, 0, tea.MouseButtonLeft, tea.MouseActionPress))
	if m.overlay != overlayNone {
		t.Error("click outside did not close")
	}
}

func TestImageModal(t *testing.T) {
	var out bytes.Buffer
	m := New(imageBook(t), state.Position{}, nil)
	m.EnableImages(&out, false, 8, 16)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	_, cmd := m.Update(runes("i"))
	run(cmd)
	if m.overlay != overlayImage || m.img.err != "" {
		t.Fatalf("overlay=%d err=%q", m.overlay, m.img.err)
	}
	if m.img.cols != 20 || m.img.rows != 5 {
		t.Errorf("fit = %dx%d want 20x5", m.img.cols, m.img.rows)
	}
	if s := out.String(); !strings.Contains(s, "\x1b_Ga=t,f=100") || !strings.Contains(s, "a=p,U=1,i=1,c=20,r=5") {
		t.Errorf("transmit output = %q", s)
	}
	v := m.View()
	if !strings.Contains(v, "\U0010EEEE") || !strings.Contains(v, "Hình 1. Một bức tranh") {
		t.Error("modal missing placeholder or caption")
	}

	out.Reset()
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	run(cmd)
	if m.img.idx != 1 || m.img.id != 2 || !strings.Contains(out.String(), "a=d,d=I,i=1") {
		t.Errorf("next image idx=%d id=%d out=%q", m.img.idx, m.img.id, out.String())
	}
	if c := m.caption(); c != "" {
		t.Errorf("second image caption = %q, want none", c)
	}

	out.Reset()
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	run(cmd)
	if m.overlay != overlayNone || !strings.Contains(out.String(), "a=d,d=I,i=2") {
		t.Errorf("close: overlay=%d out=%q", m.overlay, out.String())
	}
}

func TestImageModalWithoutGraphics(t *testing.T) {
	m := New(imageBook(t), state.Position{}, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	// click the [Image] line
	m.View()
	y := m.doc.Images[0].Line - m.top
	_, cmd := m.Update(mouse(60, y, tea.MouseButtonLeft, tea.MouseActionPress))
	run(cmd)
	v := m.View()
	if m.overlay != overlayImage || !strings.Contains(v, "can't display images") || !strings.Contains(v, "o open in viewer") {
		t.Errorf("fallback modal:\n%s", v)
	}
}

func TestInlineImages(t *testing.T) {
	var out bytes.Buffer
	m := New(imageBook(t), state.Position{}, nil)
	m.EnableImages(&out, true, 8, 16)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(cmd)

	im := m.doc.Images[0]
	if im.Cols != 20 || im.Rows != 5 {
		t.Fatalf("inline size %dx%d want natural 20x5", im.Cols, im.Rows)
	}
	if s := out.String(); !strings.Contains(s, "i=100,q=2") || !strings.Contains(s, "a=p,U=1,i=100,c=20,r=5") ||
		!strings.Contains(s, "a=p,U=1,i=101,c=20,r=5") {
		t.Errorf("inline transmit = %q", s)
	}
	v := m.View()
	if strings.Count(v, "\x1b[38;2;0;0;100m") != 5 {
		t.Errorf("want 5 placeholder rows for image 100")
	}
	if strings.Contains(v, "[Image") {
		t.Error("label shown for an inline image")
	}

	// nothing re-sent when only scrolling
	out.Reset()
	_, cmd = m.Update(runes("j"))
	run(cmd)
	if out.Len() != 0 {
		t.Errorf("re-sent on scroll: %q", out.String())
	}

	// clicking an inline image row opens the modal
	m.Update(runes("k"))
	_, cmd = m.Update(mouse(60, m.doc.Images[0].Line+2-m.top, tea.MouseButtonLeft, tea.MouseActionPress))
	run(cmd)
	if m.overlay != overlayImage || m.img.idx != 0 {
		t.Errorf("click on inline image: overlay=%d idx=%d", m.overlay, m.img.idx)
	}

	if c := string(m.CleanupGraphics()); !strings.Contains(c, "i=100") || !strings.Contains(c, "i=101") {
		t.Errorf("cleanup = %q", c)
	}
}

func TestInlineImageTallerThanScreenIsCapped(t *testing.T) {
	m := New(imageBook(t), state.Position{}, nil)
	m.EnableImages(&bytes.Buffer{}, true, 8, 40) // tall cells → 160x80 image needs 2 rows; force small screen
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 4})
	if r := m.doc.Images[0].Rows; r > m.bodyH()-2 && r != 1 {
		t.Errorf("rows %d exceed screen", r)
	}
}
