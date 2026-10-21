package kitty

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPlaceholderCellWidth(t *testing.T) {
	cell := string([]rune{0x10EEEE, diacritics[3], diacritics[7]})
	if w := ansi.StringWidth(cell + cell + cell); w != 3 {
		t.Fatalf("width = %d", w)
	}
	s := "\x1b[38;5;1m" + cell + cell + "\x1b[39m"
	if ansi.Truncate(s, 5, "") != s {
		t.Error("truncate altered placeholder row")
	}
}

func TestFit(t *testing.T) {
	cases := []struct{ w, h, mc, mr, cols, rows int }{
		{800, 400, 100, 50, 100, 25}, // width-bound
		{400, 1600, 100, 50, 25, 50}, // height-bound
		{80, 32, 100, 50, 10, 2},     // small image: natural size
	}
	for _, c := range cases {
		cols, rows := Fit(c.w, c.h, c.mc, c.mr, 8, 16)
		if cols != c.cols || rows != c.rows {
			t.Errorf("Fit(%d,%d,%d,%d) = %d,%d want %d,%d", c.w, c.h, c.mc, c.mr, cols, rows, c.cols, c.rows)
		}
	}
}

func TestTransmitChunksAndPlaceholder(t *testing.T) {
	data := make([]byte, 10000)
	out := string(Transmit(7, data, 4, 2))
	if n := strings.Count(out, "\x1b_G"); n != 5 { // 4 data chunks + placement
		t.Errorf("escape count = %d", n)
	}
	if !strings.Contains(out, "a=p,U=1,i=7,c=4,r=2") {
		t.Error("missing virtual placement")
	}
	rows := Placeholder(7, 4, 2)
	if len(rows) != 2 || ansi.StringWidth(rows[0]) != 4 || !strings.HasPrefix(rows[1], "\x1b[38;2;0;0;7m") {
		t.Errorf("placeholder rows = %q", rows)
	}
}

func TestParseReply(t *testing.T) {
	c := parseReply([]byte("\x1b_Gi=31;OK\x1b\\\x1b[6;18;9t\x1b[?62;22c"))
	if !c.Graphics || c.CellW != 9 || c.CellH != 18 {
		t.Errorf("caps = %+v", c)
	}
	c = parseReply([]byte("\x1b[?1;2c"))
	if c.Graphics || c.CellW != 0 {
		t.Errorf("plain terminal caps = %+v", c)
	}
	if !strings.HasPrefix(PlaceholderRow(0x010203, 1, 0), "\x1b[38;2;1;2;3m") {
		t.Errorf("id color = %q", PlaceholderRow(0x010203, 1, 0))
	}
}

func TestParseColorReplies(t *testing.T) {
	c := parseReply([]byte("\x1b]10;rgb:e6e6/d7d7/adad\x1b\\\x1b]11;rgb:24/24/24\x07\x1b]4;5;rgb:c6/78/dd\x1b\\\x1b[?62c"))
	if c.Colors.FG != "#e6d7ad" || c.Colors.BG != "#242424" || c.Colors.Palette[5] != "#c678dd" {
		t.Errorf("colors = %+v", c.Colors)
	}
	if c := parseReply([]byte("\x1b[?1c")); c.Colors.FG != "" || len(c.Colors.Palette) != 0 {
		t.Errorf("no replies should give no colors: %+v", c.Colors)
	}
}
