package epub

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const containerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`

func writeEPUB(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenEPUB3Nav(t *testing.T) {
	p := writeEPUB(t, map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Sách Thử</dc:title><dc:creator>A</dc:creator><dc:creator> B  C </dc:creator></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="text/ch%201.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/ch2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`,
		"OEBPS/nav.xhtml": `<html xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="landmarks"><ol><li><a href="text/ch2.xhtml">Wrong</a></li></ol></nav>
<nav epub:type="toc"><ol>
  <li><a href="text/ch%201.xhtml">Một</a>
    <ol><li><a href="text/ch%201.xhtml#s1">Một.Một</a></li></ol></li>
  <li><a href="text/ch2.xhtml"><span>Hai</span></a></li>
</ol></nav></body></html>`,
		"OEBPS/text/ch 1.xhtml": `<html><body><p>one</p></body></html>`,
		"OEBPS/text/ch2.xhtml":  `<html><body><p>two</p></body></html>`,
	})

	b, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Sách Thử" || b.Author != "A, B C" {
		t.Errorf("title = %q author = %q", b.Title, b.Author)
	}
	if title, author, err := Meta(p); err != nil || title != b.Title || author != b.Author {
		t.Errorf("Meta = %q, %q, %v", title, author, err)
	}
	if len(b.Chapters) != 2 || b.Chapters[0].Path != "OEBPS/text/ch 1.xhtml" {
		t.Fatalf("chapters = %+v", b.Chapters)
	}
	if string(b.Chapters[1].HTML) != `<html><body><p>two</p></body></html>` {
		t.Errorf("chapter html = %q", b.Chapters[1].HTML)
	}
	want := []TOCEntry{
		{Title: "Một", Path: "OEBPS/text/ch 1.xhtml", Depth: 0},
		{Title: "Một.Một", Path: "OEBPS/text/ch 1.xhtml", Fragment: "s1", Depth: 1},
		{Title: "Hai", Path: "OEBPS/text/ch2.xhtml", Depth: 0},
	}
	if len(b.TOC) != len(want) {
		t.Fatalf("toc = %+v", b.TOC)
	}
	for i := range want {
		if b.TOC[i] != want[i] {
			t.Errorf("toc[%d] = %+v, want %+v", i, b.TOC[i], want[i])
		}
	}
}

func TestOpenEPUB2NCX(t *testing.T) {
	p := writeEPUB(t, map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Old</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="c1.html" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/toc.ncx": `<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap>
  <navPoint><navLabel><text>Chapter 1</text></navLabel><content src="c1.html"/>
    <navPoint><navLabel><text>Part A</text></navLabel><content src="c1.html#a"/></navPoint>
  </navPoint>
</navMap></ncx>`,
		"OEBPS/c1.html": `<p>x</p>`,
	})

	b, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []TOCEntry{
		{Title: "Chapter 1", Path: "OEBPS/c1.html"},
		{Title: "Part A", Path: "OEBPS/c1.html", Fragment: "a", Depth: 1},
	}
	if len(b.TOC) != 2 || b.TOC[0] != want[0] || b.TOC[1] != want[1] {
		t.Errorf("toc = %+v", b.TOC)
	}
}

func TestOpenNotEPUB(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.epub")
	os.WriteFile(p, []byte("nope"), 0o644)
	if _, err := Open(p); err == nil {
		t.Fatal("expected error")
	}
}

func TestNestPartsInFlatTOC(t *testing.T) {
	toc := []TOCEntry{
		{Title: "Lời giới thiệu"}, {Title: "PHẦN I: LẠC VÀO VŨ TRỤ"}, {Title: "01. Vũ trụ hình thành"},
		{Title: "02. Hệ mặt trời"}, {Title: "Phần II: Kích thước"}, {Title: "04. Đo lường"},
	}
	nestParts(toc)
	got := []int{}
	for _, e := range toc {
		got = append(got, e.Depth)
	}
	if fmt.Sprint(got) != "[0 0 1 1 0 1]" {
		t.Errorf("depths %v", got)
	}

	// one part only, or an already nested list: untouched
	single := []TOCEntry{{Title: "Part One"}, {Title: "Chapter 1"}}
	nestParts(single)
	nested := []TOCEntry{{Title: "Part I"}, {Title: "Ch 1", Depth: 1}, {Title: "Part II"}, {Title: "Partial truths"}}
	nestParts(nested)
	if single[1].Depth != 0 || nested[3].Depth != 0 {
		t.Errorf("changed lists it shouldn't: %+v %+v", single, nested)
	}
	// "Partial" must not count as a part
	if partTitle.MatchString("Partial truths") || !partTitle.MatchString("BOOK III: Dawn") {
		t.Error("part title matcher")
	}
}
