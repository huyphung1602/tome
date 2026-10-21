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
	Chapter    int  `json:"chapter"`
	Block      int  `json:"block"`
	HideTOC    bool `json:"hide_toc,omitempty"`    // passed to the UI from Prefs
	TOCWidth   int  `json:"toc_width,omitempty"`   // passed to the UI from Prefs
	ReadHeight int  `json:"read_height,omitempty"` // passed to the UI from Prefs
	Progress   int  `json:"progress,omitempty"`    // percent read

	Bookmarks []Mark `json:"bookmarks,omitempty"`

	// Reading is seconds read per day ("2006-01-02"), for the garden.
	Reading map[string]int `json:"reading,omitempty"`
	// ReadingAdd is what the reader counted since its last save. It is
	// added to the file's Reading at save time, never written itself, so
	// two windows can't overwrite each other's time.
	ReadingAdd map[string]int `json:"-"`
	// ReadingOnly asks the save to store ReadingAdd and nothing else (the
	// reader's minute tick), so an idle window doesn't rewrite the position
	// or bookmarks another window saved.
	ReadingOnly bool `json:"-"`

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
	return writeAtomic(dir, key+".json", data)
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

// MergeReading adds seconds per day to saved ones.
func MergeReading(saved, add map[string]int) map[string]int {
	if len(add) == 0 {
		return saved
	}
	if saved == nil {
		saved = map[string]int{}
	}
	for day, s := range add {
		saved[day] += s
	}
	return saved
}

// AddReading adds seconds per day to a book's saved reading time,
// leaving the rest of its state as the file has it.
func AddReading(dir, key string, add map[string]int) error {
	if len(add) == 0 {
		return nil
	}
	p := Load(dir, key)
	p.Reading = MergeReading(p.Reading, add)
	return Save(dir, key, p)
}

// Reading collects seconds per day per book title from every state file
// except skip, including books whose file has since gone.
func Reading(dir, skip string) map[string]map[string]int {
	out := map[string]map[string]int{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.Name() == "prefs.json" || key == skip {
			continue
		}
		p := Load(dir, key)
		title := p.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path))
		}
		for day, s := range p.Reading {
			if out[day] == nil {
				out[day] = map[string]int{}
			}
			out[day][title] += s
		}
	}
	return out
}

// Prefs are UI settings shared by every book.
type Prefs struct {
	HideTOC      bool `json:"hide_toc,omitempty"`
	TOCWidth     int  `json:"toc_width,omitempty"`
	ReadHeight   int  `json:"read_height,omitempty"`   // rows; 0 = default
	Hinted       bool `json:"hinted,omitempty"`        // the "press ? for keybinds" hint was shown
	DailyMinutes int  `json:"daily_minutes,omitempty"` // the garden's goal; 0 = default
}

func LoadPrefs(dir string) Prefs {
	var p Prefs
	if data, err := os.ReadFile(filepath.Join(dir, "prefs.json")); err == nil {
		json.Unmarshal(data, &p)
	}
	return p
}

// Goal is the daily reading goal in minutes.
func (p Prefs) Goal() int {
	if p.DailyMinutes <= 0 {
		return 30
	}
	return p.DailyMinutes
}

func SavePrefs(dir string, p Prefs) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(p)
	return writeAtomic(dir, "prefs.json", data)
}

// writeAtomic replaces dir/name through a temp file of its own, so two
// tome windows saving at once never write into the same temp file.
func writeAtomic(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
