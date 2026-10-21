package web

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/huyphung1602/tome/internal/epub"
	"github.com/huyphung1602/tome/internal/render"
)

// chapterID and anchorID are the element ids chapters and the book's own
// ids get in the page, so every chapter can share one document.
func chapterID(i int) string           { return fmt.Sprintf("c%d", i) }
func anchorID(i int, id string) string { return fmt.Sprintf("c%d-%s", i, id) }

var dropTags = map[string]bool{
	"script": true, "noscript": true, "iframe": true, "object": true, "embed": true,
	"form": true, "input": true, "button": true, "textarea": true, "select": true, "base": true, "meta": true, "link": true,
}

// bookHTML renders every chapter's body into one fragment: scripts and
// event handlers removed, ids prefixed per chapter, internal links turned
// into in-page anchors, and resource URLs pointed at the server.
func bookHTML(book *epub.Book, base string) (body string, styles []string) {
	index := map[string]int{}
	for i, ch := range book.Chapters {
		index[ch.Path] = i
	}
	css := render.ParseCSS(book.CSS)
	var out strings.Builder
	for i, ch := range book.Chapters {
		// parse with tome's renderer so paragraphs carry the block numbers
		// tome uses for reading positions (data-tb)
		doc, blocks := render.BlockMap(ch, css)
		if doc == nil {
			continue
		}
		// styles inside the chapter's <head>
		var walkHead func(*html.Node)
		walkHead = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "style" && n.FirstChild != nil {
				styles = append(styles, rewriteCSS(n.FirstChild.Data, ch.Path, base+"res/"))
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walkHead(c)
			}
		}
		bodyNode := find(doc, "body")
		if head := find(doc, "head"); head != nil {
			walkHead(head)
		}
		if bodyNode == nil {
			bodyNode = doc
		}
		clean(bodyNode, i, ch.Path, base, index, blocks)
		fmt.Fprintf(&out, "<section class=\"tome-chapter\" id=\"%s\">\n", chapterID(i))
		for c := bodyNode.FirstChild; c != nil; c = c.NextSibling {
			html.Render(&out, c)
		}
		out.WriteString("\n</section>\n")
	}
	return out.String(), styles
}

func find(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, tag); f != nil {
			return f
		}
	}
	return nil
}

// clean rewrites a chapter's tree in place.
func clean(n *html.Node, chapter int, docPath, base string, index map[string]int, blocks map[*html.Node]int) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && dropTags[c.Data] {
			n.RemoveChild(c)
		} else if c.Type == html.CommentNode {
			n.RemoveChild(c)
		} else {
			clean(c, chapter, docPath, base, index, blocks)
		}
		c = next
	}
	if n.Type != html.ElementNode {
		return
	}
	var attrs []html.Attribute
	if b, ok := blocks[n]; ok {
		attrs = append(attrs, html.Attribute{Key: "data-tb", Val: strconv.Itoa(b)})
	}
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		// check the original value: rewriting would hide the scheme
		if v := strings.ToLower(strings.TrimSpace(a.Val)); strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "vbscript:") {
			continue
		}
		switch {
		case strings.HasPrefix(key, "on"):
			continue
		case key == "id" || key == "name" && n.Data == "a":
			a.Val = anchorID(chapter, a.Val)
		case key == "href" && (n.Data == "a" || n.Data == "area"):
			a.Val = linkURL(a.Val, chapter, docPath, base, index)
			if strings.HasPrefix(a.Val, "http") {
				attrs = append(attrs, html.Attribute{Key: "target", Val: "_blank"}, html.Attribute{Key: "rel", Val: "noopener"})
			}
		case key == "src" || key == "href" || key == "poster":
			a.Val = resourceURL(a.Val, docPath, base)
		case key == "srcset":
			continue
		case key == "style":
			a.Val = rewriteCSS(a.Val, docPath, base+"res/")
		}
		attrs = append(attrs, a)
	}
	n.Attr = attrs
}

// urlScheme matches any "scheme:" prefix (http:, mailto:, ftp:, kindle:, data:…).
var urlScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// external reports whether u points outside the book.
func external(u string) bool { return urlScheme.MatchString(strings.TrimSpace(u)) }

// linkURL turns a link into an in-page anchor when it points into the book.
func linkURL(href string, chapter int, docPath, base string, index map[string]int) string {
	if href == "" || external(href) {
		return href
	}
	p, frag := epub.Resolve(docPath, href)
	if p == "" {
		p = docPath
	}
	if i, ok := index[p]; ok {
		if frag != "" {
			return "#" + anchorID(i, frag)
		}
		return "#" + chapterID(i)
	}
	return base + "res/" + p
}

func resourceURL(src, docPath, base string) string {
	if src == "" || external(src) || strings.HasPrefix(src, "#") {
		return src
	}
	p, _ := epub.Resolve(docPath, src)
	return base + "res/" + p
}

var cssURL = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)(['"]?)\s*\)`)

// rewriteCSS points relative url(...) references at the server.
func rewriteCSS(css, cssPath, resBase string) string {
	return cssURL.ReplaceAllStringFunc(css, func(m string) string {
		sub := cssURL.FindStringSubmatch(m)
		u := strings.TrimSpace(sub[2])
		if external(u) || strings.HasPrefix(u, "#") {
			return m
		}
		p, _ := epub.Resolve(cssPath, u)
		return `url("` + resBase + p + `")`
	})
}
