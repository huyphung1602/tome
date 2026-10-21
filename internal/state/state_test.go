package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	if got := Load(dir, "k"); !reflect.DeepEqual(got, Position{}) {
		t.Fatalf("empty load = %+v", got)
	}
	want := Position{Chapter: 3, Block: 42, HideTOC: true, Bookmarks: []Mark{{1, 2}, {3, 4}}}
	if err := Save(dir, "k", want); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir, "k"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestKeyChangesWithSize(t *testing.T) {
	p := filepath.Join(t.TempDir(), "b.epub")
	os.WriteFile(p, []byte("a"), 0o644)
	k1, err := Key(p)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p, []byte("ab"), 0o644)
	k2, _ := Key(p)
	if k1 == k2 {
		t.Error("key should change with size")
	}
}

func TestDirHonorsXDG(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	if d, _ := Dir(); d != filepath.Join(base, "tome") {
		t.Errorf("dir = %s", d)
	}
}

func TestRecentSkipsMissingAndSortsByOpened(t *testing.T) {
	dir := t.TempDir()
	books := t.TempDir()
	a, b := filepath.Join(books, "a.epub"), filepath.Join(books, "b.epub")
	os.WriteFile(a, nil, 0o644)
	os.WriteFile(b, nil, 0o644)
	Save(dir, "1", Position{Path: a, Opened: 10})
	Save(dir, "2", Position{Path: b, Opened: 20})
	Save(dir, "3", Position{Path: filepath.Join(books, "gone.epub"), Opened: 30})
	Save(dir, "4", Position{Chapter: 1}) // old format without a path
	got := Recent(dir)
	if len(got) != 2 || got[0].Path != b || got[1].Path != a {
		t.Errorf("recent = %+v", got)
	}
}

func TestPrefsRoundTripAndNotRecent(t *testing.T) {
	dir := t.TempDir()
	if p := LoadPrefs(dir); p != (Prefs{}) {
		t.Fatalf("empty prefs = %+v", p)
	}
	SavePrefs(dir, Prefs{TOCWidth: 44, HideTOC: true})
	if p := LoadPrefs(dir); p.TOCWidth != 44 || !p.HideTOC {
		t.Errorf("prefs = %+v", p)
	}
	if r := Recent(dir); len(r) != 0 {
		t.Errorf("prefs listed as a recent book: %+v", r)
	}
}
func TestReadingMergesDeltas(t *testing.T) {
	dir := t.TempDir()
	// two windows on the same book each hand over their own delta
	for _, add := range []map[string]int{{"2026-10-21": 300}, {"2026-10-21": 120, "2026-10-22": 60}} {
		p := Load(dir, "k")
		p.Reading = MergeReading(p.Reading, add)
		if err := Save(dir, "k", p); err != nil {
			t.Fatal(err)
		}
	}
	got := Load(dir, "k").Reading
	if got["2026-10-21"] != 420 || got["2026-10-22"] != 60 {
		t.Errorf("reading = %v", got)
	}
}

func TestReadingAcrossBooks(t *testing.T) {
	dir := t.TempDir()
	Save(dir, "a", Position{Title: "A", Path: "/gone/a.epub", Reading: map[string]int{"2026-10-21": 600}})
	Save(dir, "b", Position{Title: "B", Reading: map[string]int{"2026-10-21": 60, "2026-10-20": 30}})
	SavePrefs(dir, Prefs{})
	all := Reading(dir, "")
	if all["2026-10-21"]["A"] != 600 || all["2026-10-21"]["B"] != 60 || all["2026-10-20"]["B"] != 30 {
		t.Errorf("all = %v", all) // a book whose file is gone still counts
	}
	if got := Reading(dir, "a"); got["2026-10-21"]["A"] != 0 {
		t.Errorf("skip kept a: %v", got)
	}
}

func TestOldFilesLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"chapter":2,"block":5}`), 0o644)
	os.WriteFile(filepath.Join(dir, "prefs.json"), []byte(`{"hide_toc":true}`), 0o644)
	if p := Load(dir, "old"); p.Chapter != 2 || p.Reading != nil {
		t.Errorf("old position = %+v", p)
	}
	if g := LoadPrefs(dir).Goal(); g != 30 {
		t.Errorf("default goal = %d", g)
	}
}

func TestAddReadingKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	Save(dir, "k", Position{Chapter: 9, Bookmarks: []Mark{{1, 2}}, Reading: map[string]int{"2026-10-21": 60}})
	if err := AddReading(dir, "k", map[string]int{"2026-10-21": 30}); err != nil {
		t.Fatal(err)
	}
	p := Load(dir, "k")
	if p.Chapter != 9 || len(p.Bookmarks) != 1 || p.Reading["2026-10-21"] != 90 {
		t.Errorf("after AddReading: %+v", p)
	}
}

// Saves from two windows at once must not share a temp file: every save
// leaves a whole, readable file.
func TestConcurrentSaves(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Save(dir, "k", Position{Chapter: i, Reading: map[string]int{"2026-10-21": i}})
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(dir, "k.json"))
	var p Position
	if err != nil || json.Unmarshal(data, &p) != nil {
		t.Fatalf("file broken: %v %q", err, data)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp*")); len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
}
