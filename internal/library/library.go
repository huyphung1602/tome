// Package library finds EPUB files under a directory in the background.
package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/huyphung1602/tome/internal/epub"
)

type Book struct {
	Path   string
	Title  string
	Author string
}

// Library collects books found by a background scan; it is safe to read
// while the scan runs.
type Library struct {
	mu    sync.Mutex
	books []Book
	done  bool
}

// Scan starts walking root and returns immediately.
func Scan(root string) *Library {
	l := &Library{}
	go l.scan(root)
	return l
}

// Snapshot returns the books found so far and whether the scan finished.
func (l *Library) Snapshot() ([]Book, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.books[:len(l.books):len(l.books)], l.done
}

func (l *Library) scan(root string) {
	home, _ := os.UserHomeDir()
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && skipDir(p, d.Name(), home) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(p), ".epub") {
			return nil
		}
		b := Book{Path: p}
		b.Title, b.Author, _ = epub.Meta(p)
		l.mu.Lock()
		l.books = append(l.books, b)
		l.mu.Unlock()
		return nil
	})
	l.mu.Lock()
	l.done = true
	l.mu.Unlock()
}

// skipDir leaves out places that are slow to walk and never hold books
// you'd pick by hand.
func skipDir(p, name, home string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "__pycache__", "venv", "target", "dist", "build", "testdata":
		return true
	}
	if home != "" {
		switch p {
		case filepath.Join(home, "Library"), filepath.Join(home, "go", "pkg"):
			return true
		}
	}
	// Apple Books keeps unpacked EPUBs as directories named *.epub.
	return strings.EqualFold(filepath.Ext(name), ".epub")
}
