package render

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mattn/go-runewidth"
	"golang.org/x/net/html"
)

// Code blocks are drawn in a dim frame with the language in the top edge:
//
//	╭─ c++ ──────────
//	│ int main() {
//	│ ↪ continuation of a long line
//	╰────────────────
//
// and highlighted with chroma, mapped onto the terminal theme's ANSI colors.

// codeLanguage reads a language hint from class names such as
// "language-cpp", "lang-js", "brush: java", or data-lang attributes.
func codeLanguage(n *html.Node) string {
	for node := n; node != nil; node = node.FirstChild {
		if node.Type != html.ElementNode {
			continue
		}
		for _, a := range node.Attr {
			switch a.Key {
			case "data-lang", "data-language", "data-code-language":
				return a.Val
			case "class":
				for _, c := range strings.Fields(strings.ReplaceAll(a.Val, ":", " ")) {
					for _, p := range []string{"language-", "lang-", "highlight-source-", "highlight-"} {
						if l, ok := strings.CutPrefix(c, p); ok && l != "" {
							return l
						}
					}
				}
				if i := strings.Index(a.Val, "brush:"); i >= 0 {
					if f := strings.Fields(strings.TrimPrefix(a.Val[i:], "brush:")); len(f) > 0 {
						return strings.TrimSuffix(f[0], ";")
					}
				}
			}
		}
	}
	return ""
}

// lexerFor returns the lexer named by the hint, or nil. tome doesn't guess
// a language from the code: guesses are often wrong (C++ read as GDScript),
// and wrong colors are worse than none.
func lexerFor(hint string) chroma.Lexer {
	if hint == "" {
		return nil
	}
	l := lexers.Get(hint)
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return chroma.Coalesce(l)
}

// pygmentsStyle maps Pygments' short token classes, which many technical
// books keep from their source (codehilite), to tome styles.
func pygmentsStyle(class string) (Style, bool) {
	for _, c := range strings.Fields(class) {
		switch {
		case c == "kt", c == "nc", c == "nb", c == "bp":
			return CodeType, true
		case c == "cp" || c == "k" || len(c) == 2 && c[0] == 'k':
			return CodeKeyword, true
		case c == "c" || len(c) == 2 && c[0] == 'c':
			return Faint | Italic, true
		case c == "s" || len(c) == 2 && c[0] == 's':
			return CodeString, true
		case c == "m" || c == "il" || len(c) == 2 && c[0] == 'm':
			return CodeNumber, true
		case c == "n", c == "o", c == "p", c == "w", c == "nf", c == "nv", c == "na", c == "nx", c == "nn", c == "nt", c == "err":
			return 0, true
		}
	}
	return 0, false
}

// markupLines returns the block's lines styled from its own markup:
// Pygments token classes, <b>/<strong>, <i>/<em>, and bold/italic CSS
// classes (authors often bold the part of a listing they're discussing).
// pygments reports whether any Pygments class was present.
func (r *renderer) markupLines(n *html.Node) (lines [][]Span, pygments bool) {
	lines = [][]Span{nil}
	found := false
	var walk func(*html.Node, Style)
	walk = func(n *html.Node, st Style) {
		if n.Type == html.TextNode {
			for i, part := range strings.Split(expandTabs(strings.ReplaceAll(n.Data, "\r\n", "\n")), "\n") {
				if i > 0 {
					lines = append(lines, nil)
				}
				if part != "" {
					k := len(lines) - 1
					lines[k] = append(lines[k], Span{part, st})
				}
			}
			return
		}
		if n.Type == html.ElementNode {
			if s, ok := pygmentsStyle(attr(n, "class")); ok {
				found, st = true, st&(Bold|Italic)|s
			}
			switch n.Data {
			case "b", "strong":
				st |= Bold
			case "i", "em":
				st |= Italic
			}
			st |= cssStyle(r.css.style(attr(n, "class")))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, st)
		}
	}
	walk(n, 0)
	// trim leading and trailing empty lines like the plain path does
	for len(lines) > 0 && len(lines[0]) == 0 {
		lines = lines[1:]
	}
	for len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, found
}

func tokenStyle(t chroma.TokenType) Style {
	switch {
	case t.InCategory(chroma.Comment):
		return Faint | Italic
	// exact matches: chroma's subcategories lump every keyword or name together
	case t == chroma.KeywordType, t == chroma.NameBuiltin, t == chroma.NameClass:
		return CodeType
	case t.InCategory(chroma.Keyword):
		return CodeKeyword
	case t.InSubCategory(chroma.LiteralString):
		return CodeString
	case t.InSubCategory(chroma.LiteralNumber):
		return CodeNumber
	}
	return 0
}

// highlight splits code into lines of styled spans.
func highlight(code string, lexer chroma.Lexer) [][]Span {
	lines := [][]Span{nil}
	add := func(text string, st Style) {
		for i, part := range strings.Split(text, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				k := len(lines) - 1
				lines[k] = append(lines[k], Span{part, st})
			}
		}
	}
	if lexer == nil {
		add(code, 0)
		return lines
	}
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		add(code, 0)
		return lines
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		add(tok.Value, tokenStyle(tok.Type))
	}
	return lines
}

// expandTabs replaces tabs with spaces up to the next multiple of 4.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		if r == '\n' {
			col = -1
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// codeBlock lays out a <pre> element as a framed, highlighted block.
func (r *renderer) codeBlock(n *html.Node) {
	r.flush(false)
	code := strings.Trim(expandTabs(strings.ReplaceAll(nodeText(n), "\r\n", "\n")), "\n")
	if strings.TrimSpace(code) == "" {
		return
	}
	lexer := lexerFor(codeLanguage(n))
	lines, pygments := r.markupLines(n)
	if !pygments && lexer != nil {
		lines = highlight(code, lexer)
	}
	r.pendingBlank = true
	r.startBlock()

	avail := r.avail()
	gutter := Span{"│ ", Faint}
	cont := Span{"↪ ", Faint}
	inner := max(4, avail-runewidth.StringWidth(gutter.Text))

	label := ""
	if lexer != nil {
		label = strings.ToLower(lexer.Config().Name)
	}
	top := "╭" + strings.Repeat("─", max(0, avail-1))
	if label != "" && avail > len(label)+6 {
		top = "╭─ " + label + " " + strings.Repeat("─", avail-runewidth.StringWidth(label)-4)
	}
	r.codeLine([]Span{{top, Faint}}, 0, false, true)
	for _, line := range lines {
		rows := wrapCode(line, inner, max(4, inner-runewidth.StringWidth(cont.Text)))
		for i, row := range rows {
			spans := []Span{gutter}
			if i > 0 {
				spans = append(spans, cont)
			}
			gw := spansWidth(spans)
			r.codeLine(append(spans, row...), gw, i > 0, false)
		}
	}
	r.codeLine([]Span{{"╰" + strings.Repeat("─", max(0, avail-1)), Faint}}, 0, false, true)
	r.startNode = n
	r.recordBlock()
	r.block++
	r.pendingBlank = true
}

// codeLine appends one line of a code block after any list/quote prefix.
func (r *renderer) codeLine(spans []Span, gutter int, cont, deco bool) {
	var pre []Span
	for fi := range r.frames {
		if f := &r.frames[fi]; !f.used {
			pre = append(pre, f.first)
			f.used = true
		} else {
			pre = append(pre, f.rest)
		}
	}
	r.doc.Lines = append(r.doc.Lines, Line{
		Spans: merge(append(pre, spans...)), Chapter: r.chapter, Block: r.block,
		Pre: true, Deco: deco, Gutter: spansWidth(pre) + gutter, Cont: cont,
	})
}

// wrapCode hard-wraps one code line: the first row fits width, the rest
// fit contWidth (they get a "↪" marker).
func wrapCode(line []Span, width, contWidth int) [][]Span {
	var rows [][]Span
	var row []Span
	w, limit := 0, width
	for _, s := range line {
		for _, c := range s.Text {
			cw := runewidth.RuneWidth(c)
			if w+cw > limit && w > 0 {
				rows = append(rows, merge(row))
				row, w, limit = nil, 0, contWidth
			}
			row = append(row, Span{string(c), s.Style})
			w += cw
		}
	}
	return append(rows, merge(row))
}
