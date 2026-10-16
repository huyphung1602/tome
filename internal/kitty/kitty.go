// Package kitty shows images with the kitty graphics protocol using Unicode
// placeholders: the image is transmitted once, then ordinary text cells
// (U+10EEEE plus row/column diacritics) mark where the terminal draws it.
// Because placeholders are plain text, they work inside a Bubble Tea view
// and inside multiplexers that understand them (herdr, Ghostty, kitty).
package kitty

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"regexp"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
)

const placeholder = '\U0010EEEE'

// Caps is what the terminal reported about itself.
type Caps struct {
	Graphics     bool // answered the kitty graphics query with OK
	CellW, CellH int  // cell size in pixels, 0 when unknown
}

var (
	graphicsOK = regexp.MustCompile(`\x1b_Gi=31;OK\x1b\\`)
	cellReply  = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)
	da1Reply   = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)
)

func parseReply(reply []byte) Caps {
	var caps Caps
	caps.Graphics = graphicsOK.Match(reply)
	if m := cellReply.FindSubmatch(reply); m != nil {
		caps.CellH, _ = strconv.Atoi(string(m[1]))
		caps.CellW, _ = strconv.Atoi(string(m[2]))
	}
	return caps
}

// Size returns an image's pixel size without decoding it fully.
func Size(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg.Width, cfg.Height, err
}

// Fit returns the cols x rows cell box for an image scaled to fit within
// maxCols x maxRows, keeping its aspect ratio and never upscaling.
func Fit(imgW, imgH, maxCols, maxRows, cellW, cellH int) (cols, rows int) {
	if imgW <= 0 || imgH <= 0 || maxCols <= 0 || maxRows <= 0 {
		return 0, 0
	}
	scale := min(1, float64(maxCols*cellW)/float64(imgW), float64(maxRows*cellH)/float64(imgH))
	w, h := float64(imgW)*scale, float64(imgH)*scale
	cols = max(1, min(maxCols, int(w/float64(cellW)+0.5)))
	rows = max(1, min(maxRows, int(h/float64(cellH)+0.5)))
	return cols, rows
}

// Decode returns PNG bytes (as the protocol's f=100 expects) and the
// image's pixel size. PNG input is passed through untouched.
func Decode(data []byte) (pngData []byte, w, h int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	if format == "png" {
		return data, cfg.Width, cfg.Height, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), cfg.Width, cfg.Height, nil
}

// Transmit returns the escape sequences that upload pngData as image id
// and create a virtual placement of cols x rows cells for placeholders.
func Transmit(id uint32, pngData []byte, cols, rows int) []byte {
	enc := base64.StdEncoding.EncodeToString(pngData)
	var b bytes.Buffer
	const chunk = 4096
	for i := 0; i < len(enc); i += chunk {
		end := min(i+chunk, len(enc))
		more := 0
		if end < len(enc) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=t,f=100,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, enc[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, enc[i:end])
		}
	}
	fmt.Fprintf(&b, "\x1b_Ga=p,U=1,i=%d,c=%d,r=%d,C=1,q=2\x1b\\", id, cols, rows)
	return b.Bytes()
}

// Delete frees image id and its placements.
func Delete(id uint32) []byte {
	return []byte(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id))
}

// PlaceholderRow returns one row of cols placeholder cells for image id.
// The image id is carried in the 24-bit foreground color.
func PlaceholderRow(id uint32, cols, row int) string {
	cols = min(cols, len(diacritics))
	if row >= len(diacritics) {
		return strings.Repeat(" ", cols)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", id>>16&0xff, id>>8&0xff, id&0xff)
	for c := 0; c < cols; c++ {
		sb.WriteRune(placeholder)
		sb.WriteRune(diacritics[row])
		sb.WriteRune(diacritics[c])
	}
	sb.WriteString("\x1b[39m")
	return sb.String()
}

// Placeholder returns all rows of placeholder cells for image id.
func Placeholder(id uint32, cols, rows int) []string {
	out := make([]string, rows)
	for r := range out {
		out[r] = PlaceholderRow(id, cols, r)
	}
	return out
}
