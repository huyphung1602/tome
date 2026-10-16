// Package render turns EPUB chapter HTML into word-wrapped, styled lines
// for a fixed column width, laid out as one continuous document.
package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"golang.org/x/net/html"

	"github.com/huyphung1602/tome/internal/epub"
)

type Style uint16

const (
	Bold Style = 1 << iota
	Italic
	Faint
	Underline
	// Heading levels, which the UI shows in the terminal's accent colors.
	H1
	H2
	H3
	// Code token kinds, also shown in theme colors. Inline <code> uses CodeString.
	CodeKeyword
	CodeString
	CodeNumber
	CodeType
)

type Span struct {
	Text  string
	Style Style
}

// Line is one rendered row. Chapter/Block identify the source block so a
// reading position survives re-rendering at a different width.
type Line struct {
	Spans    []Span
	Chapter  int
	Block    int
	Image    int  // 1-based index into Doc.Images when this line shows an image
	ImageRow int  // row of the image drawn on this line (inline images)
	Pre      bool // preformatted: keep line breaks and spacing when copying
	Deco     bool // decoration only (heading rule, code frame): left out when copying
	Gutter   int  // leading columns that are decoration (code gutter), dropped when copying
	Cont     bool // continues the previous line (wrapped code): joined without a newline
}

// Blank reports whether the line is an empty separator.
func (l Line) Blank() bool { return len(l.Spans) == 0 && l.Image == 0 }

// Image is an image referenced by the book, in reading order.
type Image struct {
	Src     string // path inside the zip
	Alt     string
	Caption string // from the enclosing <figure>'s <figcaption>
	Line    int    // first line of its placeholder
	// Inline layout (zero when shown as an "[Image]" label):
	Cols, Rows int // size in terminal cells
	Pad        int // left padding that centers it in the column
}

// Options control rendering. ImageFit, when set, lays images out inline:
// it returns the cell size for the image at src within maxCols, or 0 rows
// to fall back to a text label.
type Options struct {
	Width    int
	CSS      CSS // the book's class styles, from ParseCSS
	ImageFit func(src string, maxCols int) (cols, rows int)
}

func (l Line) Plain() string {
	var sb strings.Builder
	for _, s := range l.Spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

type Doc struct {
	Lines         []Line
	ChapterStart  []int
	ChapterTitles []string // first heading of each chapter, may be empty
	Images        []Image
	anchors       map[string]int
	chapterIdx    map[string]int
}

// Anchor returns the line for chapter path + fragment id (fragment may be
// empty for the chapter start), or -1 when unknown.
func (d *Doc) Anchor(chapterPath, fragment string) int {
	if fragment != "" {
		if l, ok := d.anchors[chapterPath+"#"+fragment]; ok {
			return l
		}
	}
	if i, ok := d.chapterIdx[chapterPath]; ok {
		return d.ChapterStart[i]
	}
	return -1
}

// Locate returns the first line of the given block (or the next block after it).
func (d *Doc) Locate(chapter, block int) int {
	i := sort.Search(len(d.Lines), func(i int) bool {
		l := d.Lines[i]
		return l.Chapter > chapter || (l.Chapter == chapter && l.Block >= block)
	})
	if i >= len(d.Lines) {
		return max(0, len(d.Lines)-1)
	}
	return i
}

func Render(chapters []epub.Chapter, width int) *Doc {
	return RenderWith(chapters, Options{Width: width})
}

func RenderWith(chapters []epub.Chapter, opts Options) *Doc {
	width := max(opts.Width, 10)
	d := &Doc{anchors: map[string]int{}, chapterIdx: map[string]int{}}
	r := &renderer{doc: d, width: width, fit: opts.ImageFit, css: opts.CSS, lastRule: -1}
	for i, ch := range chapters {
		if i > 0 {
			r.blank()
			r.blank()
		}
		r.chapter, r.block, r.path, r.title = i, 0, ch.Path, ""
		r.lastRule = -1
		r.pendingBlank = false
		d.ChapterStart = append(d.ChapterStart, len(d.Lines))
		d.chapterIdx[ch.Path] = i
		if root, err := html.Parse(bytes.NewReader(ch.HTML)); err == nil {
			r.walk(root)
		}
		r.flush(false)
		for _, id := range r.pendingIDs {
			d.anchors[id] = min(d.ChapterStart[i], max(0, len(d.Lines)-1))
		}
		r.pendingIDs = nil
		d.ChapterTitles = append(d.ChapterTitles, r.title)
	}
	return d
}

type frame struct {
	first, rest Span
	used        bool
}

type renderer struct {
	doc     *Doc
	width   int
	fit     func(src string, maxCols int) (cols, rows int)
	css     CSS
	chapter int
	block   int
	path    string
	title   string

	cur          []Span
	style        Style
	frames       []frame
	lists        []int // per open list: -1 unordered, else next number
	liDepth      int
	pendingBlank bool
	pendingIDs   []string
	figures      []int // Images index where each open <figure> started
	lastRule     int   // line of the rule under the latest h1, or -1
	imageRef     int   // image index (1-based) for the block being flushed
}

func (r *renderer) blank() {
	r.doc.Lines = append(r.doc.Lines, Line{Chapter: r.chapter, Block: r.block - 1})
}

func (r *renderer) text(s string) {
	{
		var sb strings.Builder
		space := false
		for _, c := range s {
			if unicode.IsSpace(c) {
				space = true
				continue
			}
			if space {
				sb.WriteByte(' ')
				space = false
			}
			sb.WriteRune(c)
		}
		if space {
			sb.WriteByte(' ')
		}
		s = sb.String()
	}
	if s != "" {
		r.cur = append(r.cur, Span{s, r.style})
	}
}

func (r *renderer) hasContent() bool {
	for _, s := range r.cur {
		if strings.TrimSpace(s.Text) != "" {
			return true
		}
	}
	return false
}

// flush lays out the buffered inline content as one block.
func (r *renderer) flush(center bool) {
	if !r.hasContent() {
		r.cur = nil
		return
	}
	r.startBlock()
	avail := r.avail()
	rows := wrapWords(r.cur, avail)
	for i, row := range rows {
		var spans []Span
		for fi := range r.frames {
			f := &r.frames[fi]
			if i == 0 && !f.used {
				spans = append(spans, f.first)
			} else {
				spans = append(spans, f.rest)
			}
		}
		if center {
			if pad := (avail - spansWidth(row)) / 2; pad > 0 {
				spans = append(spans, Span{Text: strings.Repeat(" ", pad)})
			}
		}
		spans = append(spans, row...)
		r.doc.Lines = append(r.doc.Lines, Line{Spans: merge(spans), Chapter: r.chapter, Block: r.block, Image: r.imageRef})
	}
	for fi := range r.frames {
		r.frames[fi].used = true
	}
	r.block++
	r.cur = nil
}

func (r *renderer) onlyBlanksAfter(i int) bool {
	for _, l := range r.doc.Lines[i+1:] {
		if !l.Blank() {
			return false
		}
	}
	return i < len(r.doc.Lines)
}

// gap ensures n blank lines before the next block, except at the top of
// a chapter (which already has its own spacing).
func (r *renderer) gap(n int) {
	lines := r.doc.Lines
	if len(lines) == r.doc.ChapterStart[r.chapter] {
		return
	}
	have := 0
	for i := len(lines) - 1; i >= 0 && have < n && lines[i].Blank(); i-- {
		have++
	}
	for ; have < n; have++ {
		r.blank()
	}
	r.pendingBlank = false
}

// rule draws a dim line under the heading rows starting at line first,
// as wide as the widest row and centered the same way.
func (r *renderer) rule(first int) {
	w, pad := 0, r.width
	for _, l := range r.doc.Lines[first:] {
		if l.Blank() {
			continue
		}
		text := l.Plain()
		trimmed := strings.TrimLeft(text, " ")
		w = max(w, runewidth.StringWidth(trimmed))
		pad = min(pad, len(text)-len(trimmed))
	}
	if w == 0 {
		return
	}
	last := r.doc.Lines[len(r.doc.Lines)-1]
	spans := []Span{{Text: strings.Repeat("─", w), Style: Faint}}
	if pad > 0 {
		spans = append([]Span{{Text: strings.Repeat(" ", pad)}}, spans...)
	}
	r.doc.Lines = append(r.doc.Lines, Line{Spans: spans, Chapter: last.Chapter, Block: last.Block, Deco: true})
}

// startBlock adds pending spacing and pins pending anchors to the next line.
func (r *renderer) startBlock() {
	n := len(r.doc.Lines)
	if r.pendingBlank && n > 0 && !r.doc.Lines[n-1].Blank() {
		r.blank()
	}
	r.pendingBlank = false
	for _, id := range r.pendingIDs {
		r.doc.anchors[id] = len(r.doc.Lines)
	}
	r.pendingIDs = nil
}

// avail is the text width left after blockquote/list prefixes.
func (r *renderer) avail() int {
	prefixW := 0
	for _, f := range r.frames {
		prefixW += runewidth.StringWidth(f.rest.Text)
	}
	return max(r.width-prefixW, 4)
}

func (r *renderer) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		r.text(n.Data)
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			r.walk(c)
		}
		return
	}

	for _, a := range n.Attr {
		if a.Key == "id" || (a.Key == "name" && n.Data == "a") {
			r.pendingIDs = append(r.pendingIDs, r.path+"#"+a.Val)
		}
	}

	cs := r.css.style(attr(n, "class"))
	if cs.Hidden {
		return
	}
	// bold/italic from CSS apply to everything inside the element
	if extra := cssStyle(cs); extra != 0 {
		old := r.style
		r.style |= extra
		defer func() { r.style = old }()
	}

	children := func() {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			r.walk(c)
		}
	}
	tag := n.Data
	if tag == "p" || tag == "div" {
		switch cs.headingLevel() {
		case 1:
			tag = "h1"
		case 2:
			tag = "h2"
		case 3:
			tag = "h3"
		}
	}
	withStyle := func(s Style) {
		old := r.style
		r.style |= s
		children()
		r.style = old
	}

	switch tag {
	case "head", "script", "style", "title", "noscript":
	case "svg":
		src := ""
		var find func(*html.Node)
		find = func(c *html.Node) {
			if c.Type == html.ElementNode && c.Data == "image" && src == "" {
				src = attr(c, "href")
			}
			for k := c.FirstChild; k != nil; k = k.NextSibling {
				find(k)
			}
		}
		find(n)
		r.image(src, "")
	case "img", "image":
		src := attr(n, "src")
		if src == "" {
			src = attr(n, "href")
		}
		r.image(src, attr(n, "alt"))
	case "figure":
		r.flush(false)
		r.figures = append(r.figures, len(r.doc.Images))
		children()
		r.flush(false)
		r.figures = r.figures[:len(r.figures)-1]
	case "figcaption":
		r.flush(false)
		if k := len(r.figures) - 1; k >= 0 {
			caption := strings.Join(strings.Fields(nodeText(n)), " ")
			for i := r.figures[k]; i < len(r.doc.Images); i++ {
				if r.doc.Images[i].Caption == "" {
					r.doc.Images[i].Caption = caption
				}
			}
		}
		withStyle(Italic | Faint)
		r.flush(false)
	case "br":
		r.cur = append(r.cur, Span{Text: "\n"})
	case "hr":
		r.flush(false)
		r.pendingBlank = true
		r.cur = []Span{{Text: "* * *"}}
		r.flush(true)
		r.pendingBlank = true
	case "p":
		r.flush(false)
		if r.liDepth == 0 {
			r.pendingBlank = true
		}
		children()
		r.flush(cs.Center)
		if r.liDepth == 0 {
			r.pendingBlank = true
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.flush(false)
		r.pendingBlank = true
		if tag == "h1" {
			if r.lastRule >= 0 && r.onlyBlanksAfter(r.lastRule) {
				// "Chapter 5" + title: one group, one rule under the last line
				r.doc.Lines = r.doc.Lines[:r.lastRule]
			} else {
				r.gap(2)
			}
		}
		level := map[string]Style{"h1": H1, "h2": H2, "h3": H3}[tag]
		start := len(r.cur)
		withStyle(Bold | level)
		var sb strings.Builder
		for _, s := range r.cur[start:] {
			sb.WriteString(s.Text)
		}
		text := strings.Join(strings.Fields(sb.String()), " ")
		if r.title == "" {
			r.title = text
		}
		first := len(r.doc.Lines)
		r.flush(tag == "h1" || tag == "h2" || cs.Center)
		r.lastRule = -1
		if tag == "h1" && len(r.doc.Lines) > first {
			r.lastRule = len(r.doc.Lines)
			r.rule(first)
		}
		r.pendingBlank = true
	case "blockquote":
		r.flush(false)
		r.pendingBlank = true
		bar := Span{"│ ", Faint}
		r.frames = append(r.frames, frame{first: bar, rest: bar})
		children()
		r.flush(false)
		r.frames = r.frames[:len(r.frames)-1]
		r.pendingBlank = true
	case "ul", "ol":
		r.flush(false)
		if r.liDepth == 0 {
			r.pendingBlank = true
		}
		next := -1
		if n.Data == "ol" {
			next = 1
		}
		r.lists = append(r.lists, next)
		children()
		r.flush(false)
		r.lists = r.lists[:len(r.lists)-1]
		if r.liDepth == 0 {
			r.pendingBlank = true
		}
	case "li":
		r.flush(false)
		bullet := "• "
		if k := len(r.lists) - 1; k >= 0 && r.lists[k] > 0 {
			bullet = fmt.Sprintf("%d. ", r.lists[k])
			r.lists[k]++
		}
		r.frames = append(r.frames, frame{
			first: Span{Text: bullet},
			rest:  Span{Text: strings.Repeat(" ", runewidth.StringWidth(bullet))},
		})
		r.liDepth++
		children()
		r.flush(false)
		r.liDepth--
		r.frames = r.frames[:len(r.frames)-1]
	case "pre":
		r.codeBlock(n)
	case "code", "kbd", "samp", "tt":
		withStyle(CodeString)
	case "div", "section", "article", "main", "header", "footer", "aside", "nav",
		"body", "html", "table", "tbody", "thead", "tr",
		"dl", "dt", "dd", "address", "center":
		r.flush(false)
		children()
		r.flush(cs.Center || tag == "center")
	case "td", "th":
		if r.hasContent() {
			r.text("  ")
		}
		children()
	case "em", "i", "cite", "var", "dfn":
		withStyle(Italic)
	case "strong", "b":
		withStyle(Bold)
	case "u", "ins":
		withStyle(Underline)
	default:
		children()
	}
}

func cssStyle(cs ClassStyle) Style {
	var s Style
	if cs.Bold {
		s |= Bold
	}
	if cs.Italic {
		s |= Italic
	}
	return s
}

func (r *renderer) image(src, alt string) {
	r.flush(false)
	alt = strings.Join(strings.Fields(alt), " ")
	label := "[Image]"
	if alt != "" {
		label = "[Image: " + alt + "]"
	}
	img := Image{Alt: alt}
	if src != "" {
		img.Src, _ = epub.Resolve(r.path, src)
	}
	if r.fit != nil && img.Src != "" {
		avail := r.avail()
		if cols, rows := r.fit(img.Src, avail); rows > 0 {
			r.pendingBlank = true
			r.startBlock()
			img.Cols, img.Rows, img.Pad = cols, rows, r.width-avail+(avail-cols)/2
			img.Line = len(r.doc.Lines)
			r.doc.Images = append(r.doc.Images, img)
			for row := 0; row < rows; row++ {
				r.doc.Lines = append(r.doc.Lines, Line{Chapter: r.chapter, Block: r.block, Image: len(r.doc.Images), ImageRow: row})
			}
			r.block++
			r.pendingBlank = true
			return
		}
	}
	r.cur = []Span{{label, Faint}}
	r.doc.Images = append(r.doc.Images, img)
	r.imageRef = len(r.doc.Images)
	r.flush(false)
	r.imageRef = 0
	k := len(r.doc.Images) - 1
	for i := len(r.doc.Lines) - 1; i >= 0 && r.doc.Lines[i].Image == k+1; i-- {
		r.doc.Images[k].Line = i
	}
}

func nodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func spansWidth(ss []Span) int {
	w := 0
	for _, s := range ss {
		w += runewidth.StringWidth(s.Text)
	}
	return w
}

func merge(ss []Span) []Span {
	var out []Span
	for _, s := range ss {
		if s.Text == "" {
			continue
		}
		if k := len(out) - 1; k >= 0 && out[k].Style == s.Style {
			out[k].Text += s.Text
			continue
		}
		out = append(out, s)
	}
	return out
}

type token struct {
	kind  int // 0 word, 1 space, 2 newline
	spans []Span
	width int
}

func tokenize(spans []Span) []token {
	var toks []token
	for _, s := range spans {
		for _, part := range splitKeep(s.Text) {
			switch {
			case part == "\n":
				toks = append(toks, token{kind: 2})
			case part == " ":
				toks = append(toks, token{kind: 1, spans: []Span{{" ", s.Style}}})
			default:
				w := runewidth.StringWidth(part)
				if k := len(toks) - 1; k >= 0 && toks[k].kind == 0 {
					toks[k].spans = append(toks[k].spans, Span{part, s.Style})
					toks[k].width += w
				} else {
					toks = append(toks, token{kind: 0, spans: []Span{{part, s.Style}}, width: w})
				}
			}
		}
	}
	return toks
}

// splitKeep splits s into words, single " " and "\n" separators.
func splitKeep(s string) []string {
	var out []string
	start := 0
	for i, c := range s {
		if c == ' ' || c == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			out = append(out, string(c))
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func wrapWords(spans []Span, width int) [][]Span {
	var rows [][]Span
	var line []Span
	lineW := 0
	var space *Span
	emit := func() {
		rows = append(rows, line)
		line, lineW, space = nil, 0, nil
	}
	for _, t := range tokenize(spans) {
		switch t.kind {
		case 2:
			emit()
		case 1:
			if lineW > 0 {
				sp := t.spans[0]
				space = &sp
			}
		case 0:
			sep := 0
			if space != nil {
				sep = 1
			}
			if lineW > 0 && lineW+sep+t.width > width {
				emit()
				sep = 0
			}
			if sep == 1 {
				line = append(line, *space)
				lineW++
			}
			space = nil
			for _, s := range t.spans {
				for _, r := range s.Text {
					rw := runewidth.RuneWidth(r)
					if lineW+rw > width && lineW > 0 {
						emit()
					}
					line = append(line, Span{string(r), s.Style})
					lineW += rw
				}
			}
		}
	}
	if lineW > 0 {
		emit()
	}
	for i := range rows {
		rows[i] = merge(rows[i])
	}
	return trimEmpty(rows)
}

// trimEmpty drops leading/trailing empty rows (e.g. from <br> at block edges).
func trimEmpty(rows [][]Span) [][]Span {
	for len(rows) > 0 && len(rows[0]) == 0 {
		rows = rows[1:]
	}
	for len(rows) > 0 && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
	}
	return rows
}
