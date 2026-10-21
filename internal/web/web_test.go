package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huyphung1602/tome/internal/epub"
)

func testBook(t *testing.T) *epub.Book {
	t.Helper()
	files := map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf": `<package><metadata><title>Test &amp; Book</title></metadata><manifest>
<item id="c1" href="text/one.xhtml" media-type="application/xhtml+xml"/>
<item id="c2" href="text/two.xhtml" media-type="application/xhtml+xml"/>
<item id="css" href="styles/book.css" media-type="text/css"/>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
</manifest><spine><itemref idref="c1"/><itemref idref="c2"/></spine></package>`,
		"OEBPS/nav.xhtml": `<html><body><nav epub:type="toc"><ol><li><a href="text/one.xhtml">One</a></li><li><a href="text/two.xhtml#s2">Two &lt;b&gt;</a></li></ol></nav></body></html>`,
		"OEBPS/text/one.xhtml": `<html><head><style>.x { background: url(../img/bg.png) }</style><script>alert(1)</script></head>
<body><p id="p1" onclick="evil()">Hello <a href="two.xhtml#s2">next</a> <a href="https://example.com">out</a> <a href="javascript:evil()">bad</a> <a href="kindle:pos:fid:0015">k</a> <a href="ftp://ftp.example.com/x">f</a></p>
<img src="../img/cat.png" alt="cat"/><script>alert(2)</script></body></html>`,
		"OEBPS/text/two.xhtml":  `<html><body><h2 id="s2">Two</h2><p>World</p></body></html>`,
		"OEBPS/styles/book.css": `@font-face { src: url("../fonts/f.otf") } p { color: black } .a { background: url(data:image/png;base64,xx) }`,
		"OEBPS/img/cat.png":     "\x89PNG fake",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(b))
	}
	zw.Close()
	p := filepath.Join(t.TempDir(), "t.epub")
	os.WriteFile(p, buf.Bytes(), 0o644)
	b, err := epub.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func get(t *testing.T, url string) (int, string, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
}

func TestServerPageAndResources(t *testing.T) {
	s := New()
	defer s.Close()
	base, err := s.Open(testBook(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("not on loopback: %s", base)
	}

	code, ctype, page := get(t, base)
	if code != 200 || !strings.HasPrefix(ctype, "text/html") {
		t.Fatalf("page: %d %s", code, ctype)
	}
	for _, want := range []string{
		"<title>Test &amp; Book</title>",
		`href="#c0"`, `href="#c1-s2"`, // contents links
		"Two &lt;b&gt;",          // escaped TOC title
		`id=\"c0-p1\"`,           // prefixed ids
		`href=\"#c1-s2\"`,        // internal link turned into an anchor
		`target=\"_blank\"`,      // external link opens a new tab
		`/res/OEBPS/img/cat.png`, // image served by the server
		`/res/OEBPS/img/bg.png`,  // url() in a chapter <style>
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %s", want)
		}
	}
	for _, bad := range []string{"alert(", "onclick", "evil()"} {
		if strings.Contains(page, bad) {
			t.Errorf("page still contains %q", bad)
		}
	}

	code, ctype, css := get(t, base+"css/0")
	if code != 200 || !strings.HasPrefix(ctype, "text/css") || !strings.Contains(css, `/res/OEBPS/fonts/f.otf`) || !strings.Contains(css, "data:image/png") {
		t.Errorf("css: %d %s %s", code, ctype, css)
	}
	if code, ctype, body := get(t, base+"res/OEBPS/img/cat.png"); code != 200 || ctype != "image/png" || body != "\x89PNG fake" {
		t.Errorf("image: %d %s", code, ctype)
	}
}

func TestServerRejectsWithoutToken(t *testing.T) {
	s := New()
	defer s.Close()
	base, _ := s.Open(testBook(t))
	root := base[:strings.Index(base[len("http://"):], "/")+len("http://")]
	for _, u := range []string{root + "/", root + "/wrongtoken/", root + "/res/OEBPS/img/cat.png", base + "res/missing.png", base + "css/9"} {
		if code, _, _ := get(t, u); code != 404 {
			t.Errorf("%s: %d, want 404", u, code)
		}
	}
}

func TestOpenSwitchesBookAndReusesServer(t *testing.T) {
	s := New()
	defer s.Close()
	a, b := testBook(t), testBook(t)
	b.Title = "Other"
	u1, _ := s.Open(a)
	u2, _ := s.Open(b)
	if u1 != u2 {
		t.Errorf("new server for the second book: %s vs %s", u1, u2)
	}
	if _, _, page := get(t, u2); !strings.Contains(page, "<title>Other</title>") {
		t.Error("page still shows the first book")
	}
}

func TestSyncEventsAndPositions(t *testing.T) {
	s := New()
	defer s.Close()
	book := testBook(t)
	base, _ := s.Open(book)

	_, _, page := get(t, base)
	if !strings.Contains(page, `data-tb=\"0\"`) {
		t.Error("block-starting elements aren't tagged")
	}

	got := make(chan [2]int, 1)
	s.SetOnPosition(func(c, b int) { got <- [2]int{c, b} })
	if code, _, _ := get(t, base+"pos?c=1&b=3"); code != 204 {
		t.Fatalf("pos: %d", code)
	}
	if p := <-got; p != [2]int{1, 3} {
		t.Errorf("onPos got %v", p)
	}
	if code, _, _ := get(t, base+"pos?c=x"); code != 400 {
		t.Errorf("bad pos: %d", code)
	}

	resp, err := http.Get(base + "events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	read := func() string {
		buf := make([]byte, 256)
		n, _ := resp.Body.Read(buf)
		return string(buf[:n])
	}
	// the current position comes first, then whatever tome sends
	if e := read(); !strings.Contains(e, "event: pos\ndata: 1,3") {
		t.Errorf("first event %q", e)
	}
	s.Position(2, 7)
	if e := read(); !strings.Contains(e, "data: 2,7") {
		t.Errorf("pos event %q", e)
	}
	other := testBook(t)
	s.Open(other)
	if e := read(); !strings.Contains(e, "event: book") {
		t.Errorf("book change event %q", e)
	}
}

func TestWarmAndConnected(t *testing.T) {
	s := New()
	defer s.Close()
	s.Warm(testBook(t))
	base, _ := s.Open(testBook(t))
	if s.Connected() {
		t.Fatal("connected before any page opened")
	}
	resp, err := http.Get(base + "events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// nothing is sent until there's a position, so wait for the server to register the stream
	for deadline := time.Now().Add(2 * time.Second); !s.Connected() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.Connected() {
		t.Error("an open event stream should count as connected")
	}
}

func TestBundledFonts(t *testing.T) {
	s := New()
	defer s.Close()
	base, _ := s.Open(testBook(t))
	_, _, page := get(t, base)
	if !strings.Contains(page, `font-family: "Literata"`) || !strings.Contains(page, base[strings.Index(base[7:], "/")+7:]+"fonts/literata.woff2") {
		t.Error("page doesn't declare the bundled fonts")
	}
	code, ctype, body := get(t, base+"fonts/bevietnampro-regular.woff2")
	if code != 200 || ctype != "font/woff2" || !strings.HasPrefix(body, "wOF2") {
		t.Errorf("font: %d %s %q", code, ctype, body[:min(4, len(body))])
	}
	if code, _, _ := get(t, base+"fonts/../server.go"); code != 404 {
		t.Errorf("path escape: %d", code)
	}
}

func TestGardenSync(t *testing.T) {
	s := New()
	defer s.Close()
	base, _ := s.Open(testBook(t))
	s.SetGarden(Garden{Books: map[string]map[string]int{"2026-10-21": {"A": 600}}, Goal: 30, Today: "2026-10-21"})

	goals, active := make(chan int, 1), make(chan bool, 1)
	s.SetOnGoal(func(m int) { goals <- m })
	s.SetOnActive(func() { active <- true })
	post := func(u string) int {
		resp, err := http.Post(base+u, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("goal?m=45"); code != 204 || <-goals != 45 {
		t.Errorf("goal: %d", code)
	}
	if code := post("goal?m=x"); code != 400 {
		t.Errorf("bad goal: %d", code)
	}
	if code := post("active"); code != 204 || !<-active {
		t.Errorf("active: %d", code)
	}
	if code, _, _ := get(t, base+"goal?m=40"); code != 404 { // changes state: POST only
		t.Errorf("GET goal: %d", code)
	}

	resp, err := http.Get(base + "events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// events can be longer than one read: collect them in the background
	chunks := make(chan string, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			chunks <- string(buf[:n])
			if err != nil {
				close(chunks)
				return
			}
		}
	}()
	var seen strings.Builder
	until := func(want string) bool {
		timeout := time.After(2 * time.Second)
		for !strings.Contains(seen.String(), want) {
			select {
			case c, ok := <-chunks:
				if !ok {
					return false
				}
				seen.WriteString(c)
			case <-timeout:
				return false
			}
		}
		return true
	}
	if !until("event: garden") || !until(`"goal":30`) || !until(`"today":"today 10m/30m"`) {
		t.Errorf("first events lack the garden")
	}
	s.SetGarden(Garden{Goal: 45, Today: "2026-10-21"})
	if !until(`"goal":45`) {
		t.Errorf("no garden event after SetGarden")
	}

	_, _, page := get(t, base)
	if !strings.Contains(page, `"grids":`) || !strings.Contains(page, `id="garden"`) {
		t.Error("page lacks the garden")
	}
}
