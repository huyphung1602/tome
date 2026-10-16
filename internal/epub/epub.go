// Package epub reads the parts of an EPUB file a reader needs:
// title, spine chapters in reading order, and the table of contents.
package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

type Book struct {
	Title    string
	Author   string
	Chapters []Chapter
	TOC      []TOCEntry
	CSS      string // every stylesheet in the manifest, concatenated
	file     string
}

// ReadFile returns a resource (e.g. an image) by its path inside the zip.
func (b *Book) ReadFile(name string) ([]byte, error) {
	zr, err := zip.OpenReader(b.file)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	f, err := zr.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// Resolve turns an href found in the document at docPath into a zip path
// plus fragment.
func Resolve(docPath, href string) (p, frag string) {
	return resolve(path.Dir(docPath), href)
}

type Chapter struct {
	Path string // path inside the zip
	HTML []byte
}

type TOCEntry struct {
	Title    string
	Path     string // chapter path inside the zip
	Fragment string // anchor id, may be empty
	Depth    int
}

type container struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type opf struct {
	Title    []string `xml:"metadata>title"`
	Creator  []string `xml:"metadata>creator"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		TOC      string `xml:"toc,attr"`
		Itemrefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

type ncxPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Points []ncxPoint `xml:"navPoint"`
}

type ncx struct {
	Points []ncxPoint `xml:"navMap>navPoint"`
}

// Meta reads only the title and author, for listing books quickly.
func Meta(filename string) (title, author string, err error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return "", "", err
	}
	defer zr.Close()
	pkg, _, err := readPackage(reader(zr))
	if err != nil {
		return "", "", err
	}
	return pkg.title(), pkg.author(), nil
}

func (p *opf) title() string {
	if len(p.Title) > 0 {
		return collapse(p.Title[0])
	}
	return ""
}

func (p *opf) author() string {
	var names []string
	for _, c := range p.Creator {
		if c = collapse(c); c != "" {
			names = append(names, c)
		}
	}
	return strings.Join(names, ", ")
}

// reader returns a function reading a file from the zip by name.
func reader(zr *zip.ReadCloser) func(string) ([]byte, error) {
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	return func(name string) ([]byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("missing %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
}

// readPackage locates and parses the OPF package document.
func readPackage(read func(string) ([]byte, error)) (*opf, string, error) {
	data, err := read("META-INF/container.xml")
	if err != nil {
		return nil, "", err
	}
	var c container
	if err := xml.Unmarshal(data, &c); err != nil || len(c.Rootfiles) == 0 {
		return nil, "", fmt.Errorf("invalid container.xml")
	}
	opfPath := c.Rootfiles[0].FullPath
	data, err = read(opfPath)
	if err != nil {
		return nil, "", err
	}
	var pkg opf
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, "", fmt.Errorf("invalid OPF: %w", err)
	}
	return &pkg, opfPath, nil
}

func Open(filename string) (*Book, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, fmt.Errorf("not an EPUB (zip) file: %w", err)
	}
	defer zr.Close()
	read := reader(zr)
	pkg, opfPath, err := readPackage(read)
	if err != nil {
		return nil, err
	}
	opfDir := path.Dir(opfPath)

	b := &Book{file: filename, Title: pkg.title(), Author: pkg.author()}

	hrefByID := map[string]string{}
	var navPath, ncxPath string
	for _, it := range pkg.Manifest {
		p, _ := resolve(opfDir, it.Href)
		hrefByID[it.ID] = p
		if hasWord(it.Properties, "nav") {
			navPath = p
		}
		if it.MediaType == "text/css" {
			if css, err := read(p); err == nil {
				b.CSS += string(css) + "\n"
			}
		}
		if it.MediaType == "application/x-dtbncx+xml" {
			ncxPath = p
		}
	}
	if pkg.Spine.TOC != "" {
		if p, ok := hrefByID[pkg.Spine.TOC]; ok {
			ncxPath = p
		}
	}

	for _, ref := range pkg.Spine.Itemrefs {
		p, ok := hrefByID[ref.IDRef]
		if !ok {
			continue
		}
		body, err := read(p)
		if err != nil {
			continue
		}
		b.Chapters = append(b.Chapters, Chapter{Path: p, HTML: body})
	}
	if len(b.Chapters) == 0 {
		return nil, fmt.Errorf("EPUB has no readable chapters")
	}

	if navPath != "" {
		if data, err := read(navPath); err == nil {
			b.TOC = parseNav(data, path.Dir(navPath))
		}
	}
	if len(b.TOC) == 0 && ncxPath != "" {
		if data, err := read(ncxPath); err == nil {
			b.TOC = parseNCX(data, path.Dir(ncxPath))
		}
	}
	return b, nil
}

// resolve turns an href relative to dir into a zip path plus fragment.
func resolve(dir, href string) (p, frag string) {
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href, frag = href[:i], href[i+1:]
	}
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	if href == "" {
		return "", frag
	}
	p = path.Clean(path.Join(dir, href))
	return strings.TrimPrefix(p, "./"), frag
}

func hasWord(s, w string) bool {
	for _, f := range strings.Fields(s) {
		if f == w {
			return true
		}
	}
	return false
}

func parseNCX(data []byte, dir string) []TOCEntry {
	var n ncx
	if err := xml.Unmarshal(data, &n); err != nil {
		return nil
	}
	var out []TOCEntry
	var walk func([]ncxPoint, int)
	walk = func(pts []ncxPoint, depth int) {
		for _, pt := range pts {
			p, frag := resolve(dir, pt.Content.Src)
			out = append(out, TOCEntry{Title: collapse(pt.Label), Path: p, Fragment: frag, Depth: depth})
			walk(pt.Points, depth+1)
		}
	}
	walk(n.Points, 0)
	return out
}

func parseNav(data []byte, dir string) []TOCEntry {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil
	}
	var navs []*html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "nav" {
			navs = append(navs, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if len(navs) == 0 {
		return nil
	}
	nav := navs[0]
	for _, n := range navs {
		if hasWord(attr(n, "epub:type"), "toc") || hasWord(attr(n, "role"), "doc-toc") {
			nav = n
			break
		}
	}

	var out []TOCEntry
	var walkList func(ol *html.Node, depth int)
	walkList = func(ol *html.Node, depth int) {
		for li := ol.FirstChild; li != nil; li = li.NextSibling {
			if li.Type != html.ElementNode || li.Data != "li" {
				continue
			}
			for c := li.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode {
					continue
				}
				switch c.Data {
				case "a", "span":
					p, frag := resolve(dir, attr(c, "href"))
					if title := collapse(text(c)); title != "" {
						out = append(out, TOCEntry{Title: title, Path: p, Fragment: frag, Depth: depth})
					}
				case "ol", "ul":
					walkList(c, depth+1)
				}
			}
		}
	}
	for c := nav.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "ol" || c.Data == "ul") {
			walkList(c, 0)
		}
	}
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func text(n *html.Node) string {
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

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
