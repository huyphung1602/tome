// Package state persists the reading position for each book.
package state

import (
	"cmp"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Position struct {
	Chapter  int  `json:"chapter"`
	Block    int  `json:"block"`
	HideTOC  bool `json:"hide_toc,omitempty"`  // passed to the UI from Prefs
	TOCWidth int  `json:"toc_width,omitempty"` // passed to the UI from Prefs
	Progress int  `json:"progress,omitempty"`  // percent read

	Bookmarks []Mark `json:"bookmarks,omitempty"`

	// Book details, for the recent list in the book picker.
	Path   string `json:"path,omitempty"`
	Title  string `json:"title,omitempty"`
	Author string `json:"author,omitempty"`
	Opened int64  `json:"opened,omitempty"` // unix seconds
}

// Mark is a bookmarked paragraph.
type Mark struct {
	Chapter int `json:"chapter"`
	Block   int `json:"block"`
}

// Dir is $XDG_STATE_HOME/tome, defaulting to ~/.local/state/tome.
func Dir() (string, error) {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "tome"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "tome"), nil
}

// Key identifies a book by absolute path and size.
func Key(bookPath string) (string, error) {
	abs, err := filepath.Abs(bookPath)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s:%d", abs, fi.Size())))
	return hex.EncodeToString(sum[:]), nil
}

// Load returns the saved position, or a zero Position if none exists.
func Load(dir, key string) Position {
	var p Position
	data, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err == nil {
		json.Unmarshal(data, &p)
	}
	return p
}

func Save(dir, key string, p Position) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(p)
	tmp := filepath.Join(dir, key+".json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, key+".json"))
}

// Recent returns saved positions of books that still exist, most recently
// opened first.
func Recent(dir string) []Position {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Position
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || e.Name() == "prefs.json" {
			continue
		}
		p := Load(dir, strings.TrimSuffix(e.Name(), ".json"))
		if p.Path == "" {
			continue
		}
		if _, err := os.Stat(p.Path); err != nil {
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Position) int { return cmp.Compare(b.Opened, a.Opened) })
	return out
}

// Prefs are UI settings shared by every book.
type Prefs struct {
	HideTOC  bool `json:"hide_toc,omitempty"`
	TOCWidth int  `json:"toc_width,omitempty"`
}

func LoadPrefs(dir string) Prefs {
	var p Prefs
	if data, err := os.ReadFile(filepath.Join(dir, "prefs.json")); err == nil {
		json.Unmarshal(data, &p)
	}
	return p
}

func SavePrefs(dir string, p Prefs) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(p)
	tmp := filepath.Join(dir, "prefs.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "prefs.json"))
}
