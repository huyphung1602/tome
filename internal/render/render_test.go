package render

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/huyphung1602/tome/internal/epub"
)

func plain(d *Doc) string {
	var out []string
	for _, l := range d.Lines {
		out = append(out, strings.TrimRight(l.Plain(), " "))
	}
	return strings.Join(out, "\n")
}

func one(html string, width int) *Doc {
	return Render([]epub.Chapter{{Path: "c.html", HTML: []byte(html)}}, width)
}

func TestParagraphWrapAndSpacing(t *testing.T) {
	d := one(`<body><p>The quick brown fox jumps over the lazy dog.</p><p>Second   para
	here.</p></body>`, 20)
	want := "The quick brown fox\njumps over the lazy\ndog.\n\nSecond para here."
	if got := plain(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestInlineStylesStayInWord(t *testing.T) {
	d := one(`<p>ab <b>Hel</b>lo <i>it</i></p>`, 8)
	if got := plain(d); got != "ab Hello\nit" {
		t.Errorf("got %q", got)
	}
	l := d.Lines[0]
	var bold, italic bool
	for _, s := range append(l.Spans, d.Lines[1].Spans...) {
		if s.Text == "Hel" && s.Style&Bold != 0 {
			bold = true
		}
		if s.Text == "it" && s.Style&Italic != 0 {
			italic = true
		}
	}
	if !bold || !italic {
		t.Errorf("styles lost: %+v %+v", l.Spans, d.Lines[1].Spans)
	}
}

func TestHeadingCentered(t *testing.T) {
	d := one(`<h1>Title</h1><p>body</p><h3>Sub</h3><p>x</p><h1>Next</h1>`, 11)
	want := "   Title\n   ─────\n\nbody\n\nSub\n\nx\n\n\n   Next\n   ────"
	if got := plain(d); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if st := d.Lines[0].Spans[len(d.Lines[0].Spans)-1].Style; st&Bold == 0 || st&H1 == 0 {
		t.Errorf("h1 style = %b", st)
	}
	if st := d.Lines[5].Spans[0].Style; st&H3 == 0 || st&H1 != 0 {
		t.Errorf("h3 style = %b", st)
	}
	if d.ChapterTitles[0] != "Title" {
		t.Errorf("chapter title %q", d.ChapterTitles[0])
	}
}

func TestListsHangingIndent(t *testing.T) {
	d := one(`<ul><li>alpha beta gamma</li><li>x<ol><li>one</li></ol></li></ul>`, 12)
	want := "• alpha beta\n  gamma\n• x\n  1. one"
	if got := plain(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBlockquoteAndHr(t *testing.T) {
	d := one(`<p>a</p><blockquote><p>quoted words here</p></blockquote><hr/><p>b</p>`, 12)
	want := "a\n\n│ quoted\n│ words here\n\n   * * *\n\nb"
	if got := plain(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestPreAndBrAndImg(t *testing.T) {
	d := one(`<pre>
  x = 1
  y  = 2</pre><p>line1<br/>line2</p><img alt="cat"/>`, 20)
	want := "╭───────────────────\n│   x = 1\n│   y  = 2\n╰───────────────────\n\nline1\nline2\n\n[Image: cat]"
	if got := plain(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestVietnameseAndWideWidth(t *testing.T) {
	d := one(`<p>Tiếng Việt có dấu đầy đủ</p><p>漢字漢字漢字</p>`, 10)
	want := "Tiếng Việt\ncó dấu đầy\nđủ\n\n漢字漢字漢\n字"
	if got := plain(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestAnchorsChaptersAndLocate(t *testing.T) {
	d := Render([]epub.Chapter{
		{Path: "a.html", HTML: []byte(`<p>one</p><p>two</p>`)},
		{Path: "b.html", HTML: []byte(`<p>three</p><h2 id="sec">Sec</h2><p>four <span id="mid">five</span></p>`)},
	}, 20)
	start := d.ChapterStart[1]
	if d.Lines[start].Plain() != "three" {
		t.Fatalf("chapter 1 starts at %q", d.Lines[start].Plain())
	}
	if l := d.Anchor("b.html", "sec"); strings.TrimSpace(d.Lines[l].Plain()) != "Sec" {
		t.Errorf("anchor sec -> %q", d.Lines[l].Plain())
	}
	if l := d.Anchor("b.html", "mid"); d.Lines[l].Plain() != "four five" {
		t.Errorf("anchor mid -> %q", d.Lines[l].Plain())
	}
	if l := d.Anchor("b.html", ""); l != start {
		t.Errorf("chapter anchor = %d want %d", l, start)
	}
	if l := d.Anchor("missing.html", "x"); l != -1 {
		t.Errorf("missing = %d", l)
	}
	// Locate round-trips through (chapter, block).
	for i, ln := range d.Lines {
		if ln.Plain() == "" {
			continue
		}
		if got := d.Locate(ln.Chapter, ln.Block); got != i {
			t.Errorf("Locate(%d,%d) = %d want %d", ln.Chapter, ln.Block, got, i)
		}
	}
}

func TestSkipsHeadAndScript(t *testing.T) {
	d := one(`<html><head><title>T</title><style>p{}</style></head><body><script>x()</script><p>ok</p></body></html>`, 20)
	if got := plain(d); got != "ok" {
		t.Errorf("got %q", got)
	}
}

func TestBrAtBlockEdgesTrimmed(t *testing.T) {
	d := one(`<h3><br/><br/>Title</h3><p>x</p>`, 20)
	if got := plain(d); got != "Title\n\nx" {
		t.Errorf("got %q", got)
	}
}

func TestImagesWithCaptions(t *testing.T) {
	d := Render([]epub.Chapter{{Path: "OEBPS/text/c.html", HTML: []byte(
		`<p>a</p><figure><img src="../images/x%20y.png" alt="cat"/><figcaption>Hình 1. Con <b>mèo</b></figcaption></figure>` +
			`<svg><image xlink:href="../images/z.jpg"/></svg>`)}}, 30)
	if len(d.Images) != 2 {
		t.Fatalf("images = %+v", d.Images)
	}
	im := d.Images[0]
	if im.Src != "OEBPS/images/x y.png" || im.Alt != "cat" || im.Caption != "Hình 1. Con mèo" {
		t.Errorf("image 0 = %+v", im)
	}
	if d.Lines[im.Line].Plain() != "[Image: cat]" || d.Lines[im.Line].Image != 1 {
		t.Errorf("line %d = %+v", im.Line, d.Lines[im.Line])
	}
	if d.Images[1].Src != "OEBPS/images/z.jpg" || d.Lines[d.Images[1].Line].Image != 2 {
		t.Errorf("svg image = %+v", d.Images[1])
	}
	if !strings.Contains(plain(d), "Hình 1. Con mèo") {
		t.Error("figcaption not rendered as text")
	}
}

func TestInlineImageRows(t *testing.T) {
	var gotMax int
	d := RenderWith([]epub.Chapter{{Path: "c.html", HTML: []byte(
		`<p>a</p><img src="big.png"/><p>b</p><img src="bad.svg" alt="x"/><blockquote><img src="q.png"/></blockquote>`)}},
		Options{Width: 20, ImageFit: func(src string, maxCols int) (int, int) {
			switch src {
			case "big.png":
				return 10, 3
			case "q.png":
				gotMax = maxCols
				return 4, 1
			}
			return 0, 0
		}})
	want := "a\n\n\n\n\n\nb\n\n[Image: x]\n\n"
	if got := plain(d); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	im := d.Images[0]
	if im.Cols != 10 || im.Rows != 3 || im.Pad != 5 || im.Line != 2 {
		t.Errorf("inline image = %+v", im)
	}
	for r := 0; r < 3; r++ {
		if l := d.Lines[im.Line+r]; l.Image != 1 || l.ImageRow != r || l.Blank() {
			t.Errorf("row %d = %+v", r, l)
		}
	}
	if d.Images[1].Rows != 0 || d.Lines[d.Images[1].Line].Plain() != "[Image: x]" {
		t.Errorf("fallback image = %+v", d.Images[1])
	}
	if gotMax != 18 || d.Images[2].Pad != 2+(18-4)/2 {
		t.Errorf("blockquote image maxCols=%d pad=%d", gotMax, d.Images[2].Pad)
	}
}

func TestCSSClassHeadingsAndStyles(t *testing.T) {
	css := ParseCSS(`
/* calibre-style classes */
.chap { font-size: 1.66667em; font-weight: 800; text-align: left }
p.sec { font-size: 1.29167em; font-weight: 800 }
.big-thin { font-size: 1.2em }
.em, .it { font-style: italic }
.ctr { text-align: center }
.gone { display: none }
.parent .child { font-weight: bold }
@media print { .chap { color: red } }
`)
	d := RenderWith([]epub.Chapter{{Path: "c.html", HTML: []byte(
		`<p class="chap">Chapter 5</p><p class="x sec">5.1 Hiding</p><p class="big-thin">plain</p>` +
			`<p>a <span class="it">word</span></p><p class="ctr">mid</p><p class="gone">secret</p><p class="child">c</p>`)}},
		Options{Width: 21, CSS: css})
	want := "      Chapter 5\n      ─────────\n\n5.1 Hiding\n\nplain\n\na word\n\n         mid\n\nc"
	if got := plain(d); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	styleOf := func(line int, text string) Style {
		for _, s := range d.Lines[line].Spans {
			if strings.TrimSpace(s.Text) == text {
				return s.Style
			}
		}
		t.Fatalf("no span %q on line %d: %+v", text, line, d.Lines[line].Spans)
		return 0
	}
	if styleOf(0, "Chapter 5")&(Bold|H1) != Bold|H1 || styleOf(3, "5.1 Hiding")&(Bold|H3) != Bold|H3 {
		t.Error("css headings not bold")
	}
	if styleOf(5, "plain")&Bold != 0 {
		t.Error("slightly large non-bold text should stay a paragraph")
	}
	if styleOf(7, "word")&Italic == 0 {
		t.Error("italic class not applied")
	}
	if styleOf(11, "c")&Bold != 0 {
		t.Error("descendant selector applied")
	}
	if d.ChapterTitles[0] != "Chapter 5" {
		t.Errorf("title %q", d.ChapterTitles[0])
	}
}

func TestConsecutiveH1sShareOneRule(t *testing.T) {
	d := one(`<p>x</p><h1>Chapter 5</h1><h1>Information Hiding</h1><p>body</p>`, 30)
	want := "x\n\n\n          Chapter 5\n\n      Information Hiding\n      ──────────────────\n\nbody"
	if got := plain(d); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestCodeBlockFrameHighlightAndWrap(t *testing.T) {
	d := one(`<p>see</p><pre><code class="language-go">// add two
func add(a, b int) int { return a + b + 42 } // a long line that has to wrap
s := "hi"</code></pre><p>after <code>x := 1</code> here</p>`, 40)
	lines := strings.Split(plain(d), "\n")
	if !strings.HasPrefix(lines[2], "╭─ go ─") || runewidth.StringWidth(lines[2]) != 40 {
		t.Errorf("top = %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "│ // add two") || !strings.HasPrefix(lines[5], "│ ↪ ") {
		t.Errorf("code lines:\n%s", strings.Join(lines[2:9], "\n"))
	}
	var kw, str, num, comment, inline bool
	for _, l := range d.Lines {
		for _, s := range l.Spans {
			switch {
			case s.Text == "func" && s.Style&CodeKeyword != 0:
				kw = true
			case s.Text == `"hi"` && s.Style&CodeString != 0:
				str = true
			case s.Text == "42" && s.Style&CodeNumber != 0:
				num = true
			case strings.HasPrefix(s.Text, "// add") && s.Style&Faint != 0:
				comment = true
			case s.Text == "x := 1" && s.Style&CodeString != 0:
				inline = true
			}
		}
	}
	if !kw || !str || !num || !comment || !inline {
		t.Errorf("highlight kw=%v str=%v num=%v comment=%v inline=%v", kw, str, num, comment, inline)
	}
	for _, l := range d.Lines {
		if strings.HasPrefix(l.Plain(), "│") && l.Gutter < 2 {
			t.Errorf("gutter not marked on %q", l.Plain())
		}
	}
}

func TestCodeLanguageHints(t *testing.T) {
	for html, want := range map[string]string{
		`<pre class="brush: java;">class A {}</pre>`:        "java",
		`<pre data-lang="python">print(1)</pre>`:            "python",
		`<pre><code class="lang-js">let a = 1</code></pre>`: "javascript",
		`<pre class="programlisting">roses are red</pre>`:   "",
	} {
		d := one(html, 40)
		top := d.Lines[0].Plain()
		if want == "" {
			if strings.Contains(top, "─ ") {
				t.Errorf("%s: unexpected label in %q", html, top)
			}
		} else if !strings.Contains(top, "─ "+want+" ") {
			t.Errorf("%s: top %q, want label %q", html, top, want)
		}
	}
}

func TestIdentifiersStayPlain(t *testing.T) {
	d := one(`<pre class="language-go">x := len(items)</pre>`, 40)
	for _, l := range d.Lines {
		for _, s := range l.Spans {
			if strings.TrimSpace(s.Text) == "x" && s.Style != 0 {
				t.Errorf("identifier styled %b", s.Style)
			}
		}
	}
}

func TestPygmentsClassesAndNoGuessing(t *testing.T) {
	d := one(`<pre class="calibre9"><code class="k">class</code> Terrain {
  <code class="kt">int</code> x = <code class="mi">42</code>; <code class="c1">// note</code>
}</pre><pre>class Guess { public: int x; };</pre>`, 50)
	styles := map[string]Style{}
	for _, l := range d.Lines {
		for _, s := range l.Spans {
			styles[strings.TrimSpace(s.Text)] = s.Style
		}
	}
	if styles["class"] != CodeKeyword || styles["int"] != CodeType || styles["42"] != CodeNumber || styles["// note"] != Faint|Italic {
		t.Errorf("pygments styles = %+v", styles)
	}
	if st := styles["class Guess { public: int x; };"]; st != 0 {
		t.Errorf("unhinted code was highlighted: %b", st)
	}
	if !strings.Contains(plain(d), "│ class Terrain {") || !strings.Contains(plain(d), "│   int x = 42; // note") {
		t.Errorf("text changed:\n%s", plain(d))
	}
}

func TestCodeKeepsAuthorEmphasis(t *testing.T) {
	d := one(`<pre class="programlisting">function <b class="bold">dinner</b>(food) {
  <i>return</i> food;
}</pre>`, 50)
	var bold, italic bool
	for _, l := range d.Lines {
		for _, s := range l.Spans {
			bold = bold || s.Text == "dinner" && s.Style&Bold != 0
			italic = italic || s.Text == "return" && s.Style&Italic != 0
		}
	}
	if !bold || !italic {
		t.Errorf("emphasis lost: bold=%v italic=%v\n%s", bold, italic, plain(d))
	}
}

func TestDisplayBlockSpansInHeading(t *testing.T) {
	css := ParseCSS(`.chapterindex { display: block; text-align: center }
.chaptertitle1 { display: block; text-align: center }
.dropcap { float: left; font-size: 400% }`)
	d := RenderWith([]epub.Chapter{{Path: "c.html", HTML: []byte(
		`<h1 class="calibre4"><span class="chapterindex">01</span><span class="chaptertitle1">Vũ trụ hình thành</span></h1>` +
			`<p><span class="dropcap">D</span>ù cố gắng</p>`)}}, Options{Width: 30, CSS: css})
	want := "              01\n      Vũ trụ hình thành\n      ─────────────────\n\nDù cố gắng" // (30-2)/2 and (30-17)/2
	if got := plain(d); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if d.ChapterTitles[0] != "01 Vũ trụ hình thành" {
		t.Errorf("chapter title %q", d.ChapterTitles[0])
	}
	if st := d.Lines[0].Spans[len(d.Lines[0].Spans)-1].Style; st&(Bold|H1) != Bold|H1 {
		t.Errorf("number lost heading style: %b", st)
	}
}

func TestSupAndSubUseUnicodeScripts(t *testing.T) {
	d := one(`<p>quá 10<sup class="calibre9">-30</sup> giây, 10<sup>2</sup>, CO<sub>2</sub>, 25<sup>o</sup>C, ounce<sup>1.</sup> và [6]<sup>[7]</sup></p>`, 80)
	want := "quá 10⁻³⁰ giây, 10², CO₂, 25ᵒC, ounce¹. và [6][⁷]"
	if got := plain(d); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBlockMapMatchesRenderNumbering(t *testing.T) {
	ch := epub.Chapter{Path: "c.html", HTML: []byte(`<body><h1>Title</h1><div>loose text<p>one</p></div>` +
		`<p>two <b>bold</b></p><ul><li>a</li><li>b</li></ul><pre>code</pre><img src="x.png" alt="x"/><blockquote><p>q</p></blockquote></body>`)}
	d := RenderWith([]epub.Chapter{ch}, Options{Width: 40})
	doc, nodes := BlockMap(ch, nil)
	if doc == nil {
		t.Fatal("no doc")
	}
	// the text of each mapped element must be what tome shows for that block
	byBlock := map[int]string{}
	for _, l := range d.Lines {
		if !l.Blank() && !l.Deco && byBlock[l.Block] == "" {
			byBlock[l.Block] = strings.TrimSpace(strings.Trim(l.Plain(), "│•╭─ "))
		}
	}
	seen := map[int]bool{}
	for n, b := range nodes {
		seen[b] = true
		got := strings.Join(strings.Fields(nodeText(n)), " ")
		if n.Data == "img" {
			got = "[Image: x]"
		}
		if want := byBlock[b]; want == "" || !strings.HasPrefix(got, strings.Fields(want)[0]) {
			t.Errorf("block %d: element <%s> %q, tome shows %q", b, n.Data, got, want)
		}
	}
	for b := range byBlock {
		if !seen[b] {
			t.Errorf("block %d (%q) has no element", b, byBlock[b])
		}
	}
}
