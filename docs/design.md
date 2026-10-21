# Design notes

## Goals

A terminal EPUB reader with only what reading needs: a table of contents, well-formatted text, a saved position, and images where the terminal can show them. Highlights, notes, bookmarks, search inside books, library management, sync, themes and font sizes are out of scope. The book picker is only a way to open a file.

## Layout

- **Single page.** The whole book is rendered into one list of lines and scrolled continuously across chapters.
- **Reading column.** The column is at most 72 cells wide. The sidebar and the column are centered on the screen as one group, with the sidebar a few columns from the text (2 columns of padding before its `│` border and 3 after it, then the 2-column mark margin), so wide screens don't push them apart. While the sidebar border is dragged, the group's position is frozen so the border stays under the mouse.
- **Bars.** Above the text, after two blank rows, a button bar shows the book title on the left and `t contents`, `? keybinds` and `● menu` at the column's right edge, all clickable; each label starts with its key so the buttons teach the keybinds, and when the column is too narrow they drop from the left (the menu always stays). The menu drops down from its button. Below the text, after two blank rows, the status line shows the chapter and progress (a long chapter name is cut, never the percentage), search prompts and short notices. On first run it shows "press ? for keybinds" for a few seconds.
- **Sidebar emphasis.** Entries other than the current chapter are dimmed, so the text being read is the brightest thing on screen.
- **Reading frame.** The text area is at most 36 rows tall by default and centered vertically, with the status line right under the text, so the eyes travel less on tall screens. `-`/`+` change the height by 2 rows (10 at least; past the terminal height it becomes full screen). Pages turn by the frame height with no overlap, so each page is all new text. The height is an app-wide preference in `prefs.json`. Mouse events are translated to text coordinates (rows above the text are negative and only the bar buttons react there); overlays are still placed on the whole screen.
- **Sidebar.** Defaults to 36 cells, and can be resized from 20 to 70 with `<` `>` or by dragging its border. The width is one setting for all books. Long titles wrap instead of being truncated. The sidebar shrinks so the reading area keeps at least 64 cells; when even that doesn't fit, it hides, and `t` opens the contents full screen instead.
- **Theme colors only.** Text uses bold, italic, dim, underline and reverse video. Headings add ANSI palette colors (h1 magenta, h2 blue, h3 cyan), which the terminal theme defines, so they always match it. tome uses normal rather than bright colors so headings stay readable on light themes, and `NO_COLOR` turns colors off.

## Rendering (`internal/render`)

HTML is walked once per layout width and becomes `Line`s of styled spans. Each line records its `(chapter, block)`, and the reading position is saved as that pair rather than as a line number, so it survives a re-wrap at a different width. Element ids become anchors, so a TOC entry like `chapter.xhtml#sec` maps to a line.

Formatting rules:

- `h1` is bold, centered, has a dim `────` rule under it, and gets two blank lines above. Consecutive `h1`s, such as "Chapter 5" followed by the chapter title, form one group with a single rule.
- `h2` is bold and centered, and `h3`–`h6` are bold.
- Terminals can't change font size, so heading levels are told apart by color, alignment and the rule.
- Paragraphs are separated by a blank line.
- `em` is italic and `strong` is bold.
- Blockquotes get a `│ ` prefix, and lists use `•` or `1.` with a hanging indent.
- `pre` keeps its whitespace.
- `hr` becomes `* * *`.
- `figcaption` is italic and dim.

Widths are measured in display cells, so Vietnamese and CJK text wrap correctly.

Many books (Calibre, InDesign exports) style headings with classes on `<p>` instead of using `h1`–`h6`. tome reads the book's stylesheets, but only rules whose selector is a single class (`.x` or `p.x`); descendant and other complex selectors are skipped so a style never lands on the wrong text. From those rules it takes only what a terminal can show:

- `font-weight` bold or 600 and above → bold.
- `font-style: italic` → italic.
- `text-align: center` → centered.
- `display: none` → hidden.
- `display: block` on an inline element such as `span` → its own line inside the surrounding block, so a heading built from a number span and a title span shows them on two lines and keeps the heading style.
- `font-size` on a `p` or `div`: 1.6em or larger is treated as `h1`, 1.3em or larger as `h2`, and 1.15em or larger and bold as `h3`.

## Code blocks (`render/code.go`)

A `<pre>` is drawn in a dim frame. The top edge shows the language when it's known, every line starts with a `│ ` gutter, and long lines wrap with a `↪` marker. When text is copied, the frame, gutter and markers are dropped and wrapped lines are rejoined, so code comes out exactly as written.

Highlighting, in order of preference:

1. **Pygments token classes** in the markup (`k`, `kt`, `c1`, `s2`, `mi`, …). Many technical books keep these from their source, so they give the publisher's own highlighting.
2. **A language hint** (`language-x`, `lang-x`, `brush: x`, `data-lang`): the code is tokenised with chroma.
3. **Otherwise no colors.** tome doesn't guess the language from the code. In testing, guessing read C++ as GDScript, and wrong colors are worse than none.

The author's own `<b>`/`<i>` emphasis and bold or italic CSS classes inside listings are kept. Token kinds map to ANSI palette colors: keywords blue, strings and inline `<code>` green, numbers cyan, types magenta, and comments dim italic.

## Images (`internal/kitty`)

At startup, tome sends three queries to the terminal through `/dev/tty`: a kitty graphics query, `CSI 16 t` for the cell size in pixels, and DA1. Every terminal answers DA1, so tome stops reading as soon as that reply arrives instead of waiting for a timeout.

Images use kitty **Unicode placeholders**. tome uploads an image once, with a virtual placement sized in cells. From then on, the image is ordinary text: U+10EEEE cells carrying row and column diacritics, with the image id encoded in the 24-bit foreground color. Because of that, images scroll, clip and sit under overlays exactly like text in a Bubble Tea view, and they also work through multiplexers that understand placeholders, such as herdr.

- **Inline:** an image is at most the column width and the reading-area height, and is never upscaled. Only images near the viewport are uploaded.
- **Viewer:** an image is shown at its native size, shrunk only to fit the screen. Its caption comes from `figcaption`, or else from `alt`.
- **Fallback:** when graphics aren't supported, the image appears as an `[Image: alt]` line, and the viewer shows the caption plus `o` to open the image externally.

## Input quirks

When the mouse wheel scrolls fast, especially through a multiplexer, a mouse escape sequence such as `ESC[<65;50;10M` can arrive split across reads. Bubble Tea then reports the tail as typed keys, and its `<` would shrink the sidebar. tome drops any multi-character key input made only of `<`, `[`, digits, `;` and `M`/`m` that looks like such a fragment, both in the reader and in the book picker.

## Browser view (`internal/web`)

`o`, or a click on the book title in the button bar, opens the book being read in the default browser, at the paragraph at the top of tome's screen. The launcher runs in the background and its errors are reported; the link is also copied to the clipboard in case the browser doesn't come to the front. When a book opens, tome starts a small HTTP server in the background and builds the page (well under a second even for large books), so the first `o` is instant. If a browser tab is already following tome, `o` doesn't open another tab; it moves that tab to the current paragraph and says so. The server lives as long as tome: it listens on 127.0.0.1 on a random port, and every URL carries a random 128-bit token, so other pages and users can't read the book. Opening another book from the picker swaps what the server shows, and open pages reload with it.

**Content.** Each chapter's `<body>` is rewritten for the browser: scripts, embeds, forms, comments, event handlers and `javascript:` URLs are removed; element ids get a per-chapter prefix (`c3-intro`) so chapters can share one document; links into the book become in-page anchors, links with any other scheme are left alone (http opens a new tab); images, fonts and stylesheets are served from the EPUB, with `url()` in CSS rewritten. The book sits in a shadow root with its own stylesheets, so its CSS can't break the frame around it.

**Frame.** Like tome: a button bar, the contents (dimmed except the current entry, indented by level), a reading column of about 72 characters, and a status line with chapter and progress, centered as one group. The column is at most `read_height` lines tall (36 by default) and scrolls inside the frame. `space`/`b`, arrows and `h`/`l` turn whole pages, `j`/`k` scroll a line, `t` toggles the contents.

**Themes and fonts** (⚙, saved in the browser): *terminal* uses the colors tome read from the terminal at startup (OSC 10/11 for text and background, OSC 4 for the ANSI colors tome uses for headings, code and bookmarks), falling back to paper or night when the terminal didn't answer; *paper* and *night* are fixed reading themes. Fonts are system fonts with full Vietnamese coverage: mono (like the terminal), serif (Georgia) and sans. The theme's colors and font override the book's, so every book reads the same way.

**Sync.** Positions are `(chapter, block)`, the unit tome saves. The web view parses each chapter with tome's renderer (`render.BlockMap`), which records the element holding each block's first text, and tags those elements `data-tb`, so both sides count blocks the same way; across 84 books 98% of blocks get their own element and the rest resolve to the nearest earlier one. tome reports its position after every update that changes it; the server pushes it to pages over server-sent events and the page scrolls there. The page reports the block at the top of its frame (the innermost tagged element starting at or above that line) after scrolling settles, and tome jumps there. Each side ignores the echo of a move the other side caused.

**Contents hierarchy.** When a book's contents list is completely flat, entries titled like parts ("PHẦN I", "Part Two", "Book 3", "Quyển 1") become parents of the entries after them, in tome and in the browser. Lists with real levels are left alone.

## Overlays (`internal/ui`)

The overlays follow herdr's approach. tome draws the full reading view first, then splices a bordered box over it, cutting the underlying rows at display-cell positions. Only one overlay is open at a time: the menu (anchored above `● menu`), the keybinding help, or the image viewer. While an overlay is open, it receives every key, and clicking outside it closes it.

## Book picker (`internal/library`, `ui.Picker`)

Running `tome` with no argument, or with a directory, scans for `*.epub` in the background. The scan skips hidden directories, `~/Library`, `~/go/pkg`, dependency and build directories, and Apple Books' unpacked `*.epub` directories. For each file it finds, it reads only the OPF for the title and author.

The picker uses the same frame as the reader (reading height, at most 100 columns wide, centered). It refreshes every 150 ms while the scan runs. The list starts with the five most recently opened books, taken from saved state, followed by all other books sorted by title; older recent books move there and keep their progress. The filter folds case and diacritics, and every word of the query must appear in the title, author or file name. Quitting a book with `q` returns to the picker; `ctrl+c` exits the app.

## Selection and copy (`ui/selection.go`, `internal/clipboard`)

tome captures the mouse, which disables the terminal's own selection, so it implements selection itself:

- Dragging selects by display column and reverses the selected cells. Dragging past the top or bottom edge scrolls.
- Releasing copies the selection. A press and release without movement is a click, and a second press on the same line within 400 ms selects the whole paragraph.
- The copied text is rebuilt from the rendered lines. Lines of one block are joined with spaces, blocks are separated by a newline (or a blank line when the page shows one), preformatted lines keep their breaks, and margins, blockquote bars and heading rules are dropped.
- Copying sends OSC 52 and also runs the OS clipboard command when one exists, because some multiplexers drop OSC 52.
- `Shift`+drag still reaches the terminal's native selection.

## Search (`ui/search.go`)

`/` opens a prompt in the status bar, and the view jumps to the first match as you type. `enter` keeps the matches, and `n` / `N` step through them, wrapping around; while matches are active, `n` means "next match" instead of "next chapter". `esc` in the prompt cancels and returns to where the search started; after `enter`, `esc` clears the matches.

Matching runs per paragraph, over the rendered lines joined with single spaces, so phrases match across wraps. The text is split into clusters (a base rune plus its combining marks), so books stored in decomposed Unicode match too.

Matching is "smart accent", like vim's `smartcase`: a query with no diacritics is compared after folding case and stripping marks (đ → d); a query with diacritics is compared after case folding and NFC normalization only. Every match is underlined, and the current one is reversed and bold.

## Bookmarks (`ui/bookmarks.go`)

A bookmark is a `(chapter, block)` pair, the same unit as the reading position, so it survives re-wrapping. The cursor behaves like vim's (`ui/cursor.go`): it is a line, drawn as the bookmark diamond dimmed (a hollow `◇` under `NO_COLOR`). `j`/`k` move it over text lines and the view only scrolls when it gets within `scrolloff` (3) lines of an edge. The mouse wheel scrolls the view and leaves the cursor on its line until that line would leave the screen. `ctrl+d`/`ctrl+u` move view and cursor together. Full pages put the cursor at the top of the new page (the bottom when paging back). `{`/`}` jump to paragraph starts. Jumps (chapters, contents, bookmarks, search hits, `g`/`G`) and clicks put the cursor on the target without enforcing scrolloff, so a chapter heading at the top keeps it. `B` toggles the bookmark of the cursor's paragraph, and the status line names the paragraph ("bookmarked: “…”") so a mistake is easy to spot and undo. The cursor mark hides after 3 seconds: moves that aim at a spot (`j`/`k`, `{`/`}`, `g`/`G`, clicks, jumps, `B`) show it again, while reading moves (wheel, half and full pages) leave it hidden, though the cursor still follows the view. A click without a drag in the left margin toggles the paragraph next to it; a drag that starts in the margin still selects text. Bookmarked paragraphs show a red `◆` on their first visible line. `'` lists bookmarks with chapter, snippet and position; jumping leaves two lines of context above. Bookmarks are stored in the book's state file.

## Menu

The menu lists only what applies right now, each item with its shortcut:

- keybinds (`?`)
- show or hide contents (`t`)
- search (`/`) and bookmarks (`'`)
- images (`i`), only when the book has images
- back to book list (`esc`) and quit (`ctrl+c`) when the book was opened from the picker, or just quit (`esc`) otherwise

`esc` steps back one level everywhere: it closes an overlay, leaves the contents, clears a search or a selection, and only then leaves the book (back to the picker, or out of tome when the book was opened directly). In the picker it clears the filter, then quits. `q` still leaves a book, and `ctrl+c` quits from anywhere.

## State (`internal/state`)

State lives in `$XDG_STATE_HOME/tome/<sha1(abs path:size)>.json` and holds the chapter, block, percent read, and the book's path, title, author and last-opened time for the picker's recent list. Sidebar width and visibility are app-wide and live in `prefs.json`. tome writes it on quit and on every jump.
