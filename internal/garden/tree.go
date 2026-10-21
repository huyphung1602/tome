package garden

import (
	"fmt"
	"strings"
)

// MaxLevel is the biggest tree: flowers and fruit.
const MaxLevel = 6

// Level sizes a day's tree from its reading time against the daily goal:
// 0 bare ground, 1 sprout, 2 sapling, 3 tree (goal met), 4 big tree,
// 5 flowers (twice the goal), 6 flowers and fruit (four times).
func Level(secs, goalMin int) int {
	if secs <= 0 {
		return 0
	}
	r := float64(secs) / float64(goalMin*60)
	switch {
	case r < 0.5:
		return 1
	case r < 1:
		return 2
	case r < 1.5:
		return 3
	case r < 2:
		return 4
	case r < 4:
		return 5
	}
	return MaxLevel
}

// Labels are the legend's minutes for levels 1 to MaxLevel.
func Labels(goalMin int) [MaxLevel]string {
	g := goalMin
	return [MaxLevel]string{
		"<" + minutes(g/2),
		span(g/2, g),
		span(g, g*3/2),
		span(g*3/2, g*2),
		span(g*2, g*4),
		minutes(g*4) + "+",
	}
}

// span is a range of minutes, written once when both ends share a unit.
func span(a, b int) string {
	switch {
	case b < 60:
		return fmt.Sprintf("%d–%dm", a, b)
	case a >= 60 && a%60 == 0 && b%60 == 0:
		return fmt.Sprintf("%d–%dh", a/60, b/60)
	}
	return minutes(a) + "–" + minutes(b)
}

func minutes(m int) string {
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", m)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

// FormatMinutes writes seconds as whole minutes: 34m, 1h, 1h20.
func FormatMinutes(secs int) string { return minutes(secs / 60) }

// Pixel art, 9 wide and 10 tall: . empty, L light leaf, g leaf, T trunk,
// F flower, R fruit, d ground. Two pixel rows make one terminal row.
var grids = [MaxLevel + 1][10]string{
	{".........", ".........", ".........", ".........", ".........", ".........", ".........", ".........", ".........", "...ddd..."},
	{".........", ".........", ".........", ".........", ".........", ".........", ".........", "..LL.gg..", "....g....", "....g...."},
	{".........", ".........", ".........", ".........", ".........", "....L....", "...LLg...", "...Lgg...", "....T....", "....T...."},
	{".........", ".........", "....L....", "...LLg...", "..LLggg..", "..Lgggg..", "...ggg...", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LLLLg..", ".LLLgggg.", "LLggggggg", "Lgggggggg", ".ggggggg.", "..ggggg..", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LFLLg..", ".LLLgFgg.", "LLgggggFg", "LgFgggggg", ".ggggFgg.", "..ggggg..", "....T....", "....T....", "...TTT..."},
	{"...LLL...", "..LFLLg..", ".LLLgFgg.", "LLggRggFg", "LgFgggRgg", ".gRggFgg.", "..ggRgg..", "....T....", "....T....", "...TTT..."},
}

// palette maps pixels to ANSI colors, so the terminal's theme picks shades.
var palette = map[byte]int{'g': 2, 'L': 10, 'T': 3, 'F': 13, 'R': 9, 'd': 8}

// Tree draws a level as 5 rows of 9 cells with half blocks: the top pixel
// is the foreground of ▀, the bottom one its background.
func Tree(level int, color bool) []string {
	g := grids[max(0, min(level, MaxLevel))]
	rows := make([]string, 0, 5)
	for r := 0; r < 10; r += 2 {
		var sb strings.Builder
		for c := 0; c < 9; c++ {
			sb.WriteString(cell(g[r][c], g[r+1][c], color))
		}
		rows = append(rows, sb.String())
	}
	return rows
}

func cell(top, bot byte, color bool) string {
	switch {
	case top == '.' && bot == '.':
		return " "
	case !color && (top == 'R' || bot == 'R'):
		return "o" // without color, flowers and fruit keep their own marks
	case !color && (top == 'F' || bot == 'F'):
		return "*"
	case !color && top == '.':
		return "▄"
	case !color && bot == '.':
		return "▀"
	case !color:
		return "█"
	case top == bot:
		return sgr(fg(palette[top])) + "█\x1b[0m"
	case top == '.':
		return sgr(fg(palette[bot])) + "▄\x1b[0m"
	case bot == '.':
		return sgr(fg(palette[top])) + "▀\x1b[0m"
	}
	return sgr(fg(palette[top]), bg(palette[bot])) + "▀\x1b[0m"
}

func fg(n int) int {
	if n < 8 {
		return 30 + n
	}
	return 90 + n - 8
}

func bg(n int) int { return fg(n) + 10 }

func sgr(codes ...int) string {
	s := make([]string, len(codes))
	for i, c := range codes {
		s[i] = fmt.Sprint(c)
	}
	return "\x1b[" + strings.Join(s, ";") + "m"
}
