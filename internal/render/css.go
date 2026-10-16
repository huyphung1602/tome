package render

import (
	"regexp"
	"strconv"
	"strings"
)

// ClassStyle is what tome takes from a book's CSS for one class: only the
// properties a terminal can show.
type ClassStyle struct {
	Bold, Italic, Center, Hidden bool
	Size                         float64 // font-size in em; 0 when unset
}

// CSS maps class names to their styles.
type CSS map[string]ClassStyle

var (
	cssComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	simpleClass = regexp.MustCompile(`^[a-zA-Z0-9]*\.([\w-]+)$`)
)

// ParseCSS reads rules whose selectors are a single class, optionally on an
// element (".x", "p.x"). Descendant and other complex selectors are skipped
// so styles never land on the wrong text.
func ParseCSS(src string) CSS {
	css := CSS{}
	src = cssComment.ReplaceAllString(src, "")
	for _, rule := range strings.Split(src, "}") {
		sel, body, ok := strings.Cut(rule, "{")
		if !ok || strings.Contains(sel, "@") {
			continue
		}
		var decl ClassStyle
		for _, d := range strings.Split(body, ";") {
			k, v, ok := strings.Cut(d, ":")
			if !ok {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important")))
			switch k {
			case "font-weight":
				n, err := strconv.Atoi(v)
				decl.Bold = v == "bold" || v == "bolder" || err == nil && n >= 600
			case "font-style":
				decl.Italic = v == "italic" || v == "oblique"
			case "text-align":
				decl.Center = v == "center"
			case "display":
				decl.Hidden = v == "none"
			case "font-size":
				decl.Size = fontSize(v)
			}
		}
		for _, s := range strings.Split(sel, ",") {
			m := simpleClass.FindStringSubmatch(strings.TrimSpace(s))
			if m == nil {
				continue
			}
			c := css[m[1]]
			c.Bold = c.Bold || decl.Bold
			c.Italic = c.Italic || decl.Italic
			c.Center = c.Center || decl.Center
			c.Hidden = c.Hidden || decl.Hidden
			if decl.Size > 0 {
				c.Size = decl.Size
			}
			css[m[1]] = c
		}
	}
	return css
}

// fontSize converts a CSS font-size to em, assuming a 16px base.
func fontSize(v string) float64 {
	switch v {
	case "large":
		return 1.2
	case "x-large":
		return 1.5
	case "xx-large", "xxx-large":
		return 2
	}
	for unit, scale := range map[string]float64{"rem": 1, "em": 1, "%": 0.01, "px": 1.0 / 16, "pt": 1.0 / 12} {
		if num, ok := strings.CutSuffix(v, unit); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(num), 64); err == nil {
				return f * scale
			}
		}
	}
	return 0
}

// style merges the styles of every class in a class attribute.
func (c CSS) style(classAttr string) ClassStyle {
	var out ClassStyle
	for _, name := range strings.Fields(classAttr) {
		s, ok := c[name]
		if !ok {
			continue
		}
		out.Bold = out.Bold || s.Bold
		out.Italic = out.Italic || s.Italic
		out.Center = out.Center || s.Center
		out.Hidden = out.Hidden || s.Hidden
		if s.Size > 0 {
			out.Size = s.Size
		}
	}
	return out
}

// headingLevel guesses a heading from paragraph styling: 1 for very large
// text, 2 for large text, 3 for slightly larger bold text, 0 otherwise.
func (s ClassStyle) headingLevel() int {
	switch {
	case s.Size >= 1.6:
		return 1
	case s.Size >= 1.3:
		return 2
	case s.Size >= 1.15 && s.Bold:
		return 3
	}
	return 0
}
