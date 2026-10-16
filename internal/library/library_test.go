package library

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestScanFindsEpubsAndSkipsNoise(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		"a.epub", "sub/deep/B.EPUB", "notes.txt",
		".hidden/x.epub", "node_modules/pkg/y.epub", "src/testdata/z.epub", "Unpacked.epub/content.opf",
	} {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("not really a zip"), 0o644)
	}
	l := Scan(root)
	var books []Book
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		var done bool
		if books, done = l.Snapshot(); done {
			break
		}
	}
	var got []string
	for _, b := range books {
		rel, _ := filepath.Rel(root, b.Path)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "a.epub" || got[1] != "sub/deep/B.EPUB" {
		t.Errorf("found %v", got)
	}
}
