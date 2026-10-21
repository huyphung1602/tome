// Command tome is a minimal terminal EPUB reader.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huyphung1602/tome/internal/clipboard"
	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/kitty"
	"github.com/huyphung1602/tome/internal/library"
	"github.com/huyphung1602/tome/internal/state"
	"github.com/huyphung1602/tome/internal/ui"
	"github.com/huyphung1602/tome/internal/web"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `usage: tome [book.epub | directory]

  With no argument, tome lists recently read books and every EPUB under
  your home directory; with a directory, only books under it.

  j/k ↓/↑      scroll           space/b    page down/up
  g/G          start/end        n/p        next/prev chapter
  t            toggle contents  tab        focus contents (enter jumps)
  < / >        contents narrower/wider (or drag its border)
  i            view image on screen (or click it)
  m  ?         menu, keybinds
  q            save position, back to the book list (ctrl+c quits)

  TOME_IMAGES=inline|modal|off  override image support detection

  -v, --version  print version`

func main() {
	args := os.Args[1:]
	if len(args) == 1 && (args[0] == "-v" || args[0] == "--version") {
		fmt.Println("tome", buildVersion())
		return
	}
	if len(args) > 1 || len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(args); err != nil {
		fmt.Fprintln(os.Stderr, "tome:", err)
		os.Exit(1)
	}
}

// buildVersion prefers the release version, then the module version
// recorded by `go install ...@vX`, then "dev".
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

type app struct {
	web      *web.Server // started on the first "open in browser", shared by every book
	stateDir string
	caps     kitty.Caps
	images   string // TOME_IMAGES
}

func run(args []string) error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	a := &app{web: web.New(), stateDir: dir, images: os.Getenv("TOME_IMAGES")}
	defer a.web.Close()
	// also asks for the terminal's colors, which the browser view follows
	a.caps = kitty.Probe(time.Second)
	a.web.SetTheme(web.Theme{FG: a.caps.Colors.FG, BG: a.caps.Colors.BG, Palette: a.caps.Colors.Palette})

	home, _ := os.UserHomeDir()
	root := home
	if len(args) == 1 {
		fi, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			_, err := a.read(args[0], false)
			return err
		}
		if root, err = filepath.Abs(args[0]); err != nil {
			return err
		}
	}
	if root == "" {
		return fmt.Errorf("no home directory; pass a book or a directory")
	}

	lib := library.Scan(root)
	notice := ""
	for {
		p := ui.NewPicker(lib, state.Recent(dir), root, home, notice, state.LoadPrefs(dir).ReadHeight)
		if _, err := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
			return err
		}
		if p.Chosen == "" {
			return nil
		}
		notice = ""
		quitAll, err := a.read(p.Chosen, true)
		if err != nil {
			notice = "couldn't open " + filepath.Base(p.Chosen) + ": " + err.Error()
		}
		if quitAll {
			return nil
		}
	}
}

// read opens one book until the user closes it; quitAll reports ctrl+c.
// fromList means q returns to the book picker.
func (a *app) read(file string, fromList bool) (quitAll bool, err error) {
	book, err := epub.Open(file)
	if err != nil {
		return false, err
	}
	key, err := state.Key(file)
	if err != nil {
		return false, err
	}
	abs, _ := filepath.Abs(file)
	opened := time.Now().Unix()
	// sidebar width and visibility are app-wide, not per book
	pos := state.Load(a.stateDir, key)
	prefs := state.LoadPrefs(a.stateDir)
	pos.HideTOC, pos.TOCWidth, pos.ReadHeight = prefs.HideTOC, prefs.TOCWidth, prefs.ReadHeight
	hinted := prefs.Hinted
	var saveErr error
	m := ui.New(book, pos, func(p state.Position) {
		if err := state.SavePrefs(a.stateDir, state.Prefs{HideTOC: p.HideTOC, TOCWidth: p.TOCWidth, ReadHeight: p.ReadHeight, Hinted: true}); err != nil {
			saveErr = err
		}
		p.HideTOC, p.TOCWidth, p.ReadHeight = false, 0, 0
		p.Path, p.Title, p.Author, p.Opened = abs, book.Title, book.Author, opened
		if err := state.Save(a.stateDir, key, p); err != nil {
			saveErr = err
		}
	})
	m.SetBookList(fromList)
	a.web.SetReadHeight(prefs.ReadHeight)
	m.SetWebSync(a.web.Position)
	a.web.Warm(book) // the first "open in browser" shouldn't wait for the server
	m.SetWeb(func(chapter, block int) (bool, error) {
		url, err := a.web.Open(book)
		if err != nil {
			return false, err
		}
		url = fmt.Sprintf("%s#p%d-%d", url, chapter, block) // opens at this paragraph
		clipboard.Write(url)                                // a fallback when the browser doesn't come to the front
		if a.web.Connected() {
			// a tab already follows tome; don't open another one
			a.web.Position(chapter, block)
			return true, nil
		}
		return false, web.OpenBrowser(url)
	})
	if !hinted { // first run: say where the keys are
		m.ShowHint()
		prefs.Hinted = true
		state.SavePrefs(a.stateDir, prefs)
	}
	m.SetClipboard(clipboard.Write)
	switch a.images {
	case "off":
	case "inline", "modal":
		m.EnableImages(os.Stdout, a.images == "inline", a.caps.CellW, a.caps.CellH)
	default:
		if a.caps.Graphics {
			m.EnableImages(os.Stdout, true, a.caps.CellW, a.caps.CellH)
		}
	}
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	// the browser's scrolling moves the book here too
	a.web.SetOnPosition(func(chapter, block int) { prog.Send(ui.WebPos{Chapter: chapter, Block: block}) })
	_, err = prog.Run()
	a.web.SetOnPosition(nil)
	os.Stdout.Write(m.CleanupGraphics())
	if err != nil {
		return true, err
	}
	if saveErr != nil {
		return true, fmt.Errorf("saving position: %w", saveErr)
	}
	return m.QuitAll(), nil
}
