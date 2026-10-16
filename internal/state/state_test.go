package state

import (
	"os"
	"path/filepath"
	"reflect"
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
