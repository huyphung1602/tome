package web

import "embed"

// Reading fonts with full Vietnamese support, subset to Latin and
// Vietnamese and bundled so every machine reads with the same type:
// Literata (serif, variable weight) and Be Vietnam Pro (sans). Both are
// under the SIL Open Font License; see fonts/OFL-*.txt.
//
//go:embed fonts/*.woff2
var fontFiles embed.FS

// fontFaces declares the bundled fonts; base is the server's URL prefix.
func fontFaces(base string) string {
	face := func(family, file, weight, style string) string {
		return `@font-face { font-family: "` + family + `"; src: url("` + base + `fonts/` + file + `") format("woff2"); ` +
			`font-weight: ` + weight + `; font-style: ` + style + `; font-display: swap; }` + "\n"
	}
	return face("Literata", "literata.woff2", "200 900", "normal") +
		face("Literata", "literata-italic.woff2", "200 900", "italic") +
		face("Be Vietnam Pro", "bevietnampro-regular.woff2", "300 500", "normal") +
		face("Be Vietnam Pro", "bevietnampro-italic.woff2", "300 500", "italic") +
		face("Be Vietnam Pro", "bevietnampro-semibold.woff2", "600 800", "normal") +
		face("Be Vietnam Pro", "bevietnampro-semibolditalic.woff2", "600 800", "italic")
}
