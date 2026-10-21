// Package web serves the book being read to a browser, so it can be read
// with its original HTML, CSS and images. The server listens on loopback
// only, on a random port, behind a random token in the URL.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/huyphung1602/tome/internal/epub"
)

type Server struct {
	mu    sync.Mutex
	book  *epub.Book
	page  []byte // reader page for book, built on first request
	token string
	base  string // http://127.0.0.1:port/token/
	srv   *http.Server

	theme      Theme
	readHeight int
	onPos      func(chapter, block int) // the browser scrolled
	pos        string                   // last position sent to browsers, "chapter,block"
	clients    map[chan string]bool     // open event streams
}

// Theme is the terminal's colors, for the "terminal" theme in the browser.
type Theme struct {
	FG, BG  string         // "#rrggbb", "" when unknown
	Palette map[int]string // ANSI colors 1-6
}

func New() *Server { return &Server{clients: map[chan string]bool{}} }

// SetTheme sets the terminal colors the page can follow.
func (s *Server) SetTheme(t Theme) { s.mu.Lock(); s.theme, s.page = t, nil; s.mu.Unlock() }

// SetReadHeight sets the reading frame height in lines (0 = default).
func (s *Server) SetReadHeight(n int) {
	s.mu.Lock()
	if n != s.readHeight {
		s.readHeight, s.page = n, nil
	}
	s.mu.Unlock()
}

// SetOnPosition is called when the reader scrolls in the browser.
func (s *Server) SetOnPosition(fn func(chapter, block int)) { s.mu.Lock(); s.onPos = fn; s.mu.Unlock() }

// Position tells open pages to show chapter/block.
func (s *Server) Position(chapter, block int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pos = fmt.Sprintf("%d,%d", chapter, block)
	s.broadcast("pos", s.pos)
}

// broadcast sends an event to every open page without blocking.
func (s *Server) broadcast(event, data string) {
	msg := "event: " + event + "\ndata: " + data + "\n\n"
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// Open serves book (starting the server on first use) and returns the URL
// of its reader page.
func (s *Server) Open(book *epub.Book) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.book != book {
		changed := s.book != nil
		s.book, s.page, s.pos = book, nil, ""
		if changed {
			s.broadcast("book", "") // open pages reload with the new book
		}
	}
	if s.srv != nil {
		return s.base, nil
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	s.token = hex.EncodeToString(tok)
	s.base = fmt.Sprintf("http://%s/%s/", ln.Addr(), s.token)
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)
	return s.base, nil
}

// Warm starts the server for book and builds its page in the background,
// so the first "open in browser" is instant.
func (s *Server) Warm(book *epub.Book) {
	if _, err := s.Open(book); err != nil {
		return
	}
	go s.readerPage(book)
}

// Connected reports whether a page is open in a browser right now.
func (s *Server) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients) > 0
}

// Close stops the server.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	book, token := s.book, s.token
	s.mu.Unlock()
	rest, ok := strings.CutPrefix(r.URL.Path, "/"+token+"/")
	if !ok || token == "" || book == nil || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch {
	case rest == "":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.readerPage(book))
	case rest == "events":
		s.events(w, r)
	case rest == "pos":
		var c, b int
		if _, err := fmt.Sscanf(r.URL.Query().Get("c")+" "+r.URL.Query().Get("b"), "%d %d", &c, &b); err != nil {
			http.Error(w, "bad position", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.pos = fmt.Sprintf("%d,%d", c, b)
		fn := s.onPos
		s.mu.Unlock()
		if fn != nil {
			fn(c, b)
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(rest, "css/"):
		var i int
		if _, err := fmt.Sscanf(rest, "css/%d", &i); err != nil || i < 0 || i >= len(book.Styles) {
			http.NotFound(w, r)
			return
		}
		st := book.Styles[i]
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write([]byte(rewriteCSS(st.Text, st.Path, "/"+token+"/res/")))
	case strings.HasPrefix(rest, "res/"):
		name := strings.TrimPrefix(rest, "res/")
		data, err := book.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if t := mime.TypeByExtension(path.Ext(name)); t != "" {
			w.Header().Set("Content-Type", t)
		}
		w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) readerPage(book *epub.Book) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.page == nil || s.book != book {
		s.page = buildPage(book, "/"+s.token+"/", s.theme, s.readHeight)
	}
	return s.page
}

// events streams positions (and book changes) to a page.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch := make(chan string, 16)
	s.mu.Lock()
	s.clients[ch] = true
	pos := s.pos
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.clients, ch); s.mu.Unlock() }()
	if pos != "" {
		fmt.Fprintf(w, "event: pos\ndata: %s\n\n", pos)
	}
	fl.Flush()
	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// OpenBrowser opens url in the system's default browser and waits for the
// launcher, so a failure is reported instead of silently ignored.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
