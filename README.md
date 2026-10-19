# tome

[![ci](https://github.com/huyphung1602/tome/actions/workflows/ci.yml/badge.svg)](https://github.com/huyphung1602/tome/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/huyphung1602/tome)](https://github.com/huyphung1602/tome/releases)
[![license](https://img.shields.io/github/license/huyphung1602/tome)](LICENSE)

A minimal EPUB reader for the terminal.

- One continuous scroll in a reading frame of at most 72 characters by 36 lines, centered on screen
- A table of contents sidebar: click to jump, drag the border to resize
- Remembers where you stopped in every book
- A book picker that finds every EPUB in your home folder
- Inline images on [Ghostty](https://ghostty.org)
- Follows your terminal theme: no colors of its own except theme accents on headings
- Framed code blocks with syntax highlighting in your theme's colors
- Search inside the book, with or without Vietnamese accents
- Bookmarks on paragraphs
- Select text with the mouse to copy clean paragraphs

## Install

macOS, Linux and FreeBSD. tome is a single binary, so you don't need Go or anything else:

```sh
curl -fsSL https://raw.githubusercontent.com/huyphung1602/tome/main/scripts/install.sh | sh
```

The script downloads the latest release for your OS and CPU, checks its checksum, and installs `tome` to `~/.local/bin`. To install somewhere else, set `TOME_INSTALL_DIR`; to install a specific version, set `TOME_VERSION=v0.1.0`.

You can also download an archive from [Releases](https://github.com/huyphung1602/tome/releases) and put `tome` (or `tome.exe` on Windows) on your `PATH`. If macOS refuses to open a binary downloaded with a browser, run `xattr -d com.apple.quarantine tome` once.

To update, run the install script again. To uninstall, delete the binary (`rm ~/.local/bin/tome`) and, optionally, your saved positions (`rm -r ~/.local/state/tome`).

If you have Go 1.26 or later:

```sh
go install github.com/huyphung1602/tome/cmd/tome@latest
```

## Usage

```sh
tome                 # pick a book from your home folder
tome ~/Books         # pick a book from a folder
tome book.epub       # open a book directly
```

In the book picker, the five books you opened most recently come first, then every other book (with your progress, if you started it). Type to filter by title, author or file name (accents are ignored). Press `enter` to open a book and `esc` to quit.

Above the text, `t contents` shows or hides the table of contents, `? keybinds` lists every keybinding, and `● menu` opens the menu. Each label starts with its key.

| Key | Action |
|---|---|
| `j` `k` / `↓` `↑` | move the cursor (a dim ◆ in the margin that hides after a few seconds); the page scrolls at the edges |
| `space` `b`, `l` `h`, `→` `←` | page down / up |
| `g` `G` | start / end |
| `n` `p` | next / previous chapter |
| `/` | search; then `n` `N` for next / previous match, `esc` to clear |
| `{` `}` | previous / next paragraph |
| `B` | bookmark or unbookmark the paragraph the cursor is in |
| `'` | list bookmarks (`enter` jumps, `d` deletes) |
| `t` | show / hide contents |
| `tab` | focus contents (`enter` jumps) |
| `<` `>` | resize contents |
| `-` `+` | shorter / taller reading frame (`+` past the screen height = full screen) |
| `i` | view the image on screen |
| `m` `?` | menu, all keybinds |
| `q` | back to the book list |
| `ctrl+c` | quit |

**Search** ignores case. A query typed without accents also ignores accents (`ban` finds *bạn*, *bàn*, *bán*); a query with accents matches them exactly. Phrases match across line breaks.

**Mouse:** click an entry in the contents or an image, click the left margin next to a paragraph to bookmark it, drag the `│` border to resize the contents, and scroll with the wheel. Drag over text to select it; it's copied when you let go. Double-click to copy a paragraph. To use your terminal's own selection, hold `Shift` while you drag.

## Images

tome asks the terminal at startup whether it supports the [kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/).

| Terminal | Images |
|---|---|
| Ghostty (also inside herdr) | inline, tested |
| kitty, WezTerm | should work, untested |
| others | shown as `[Image: …]`; press `i` for the caption and `o` to open the image in your viewer |

To override detection, set `TOME_IMAGES=inline|modal|off`.

## Configuration

There is no config file.

- **`TOME_IMAGES`**: `inline`, `modal` or `off`.
- **`NO_COLOR`**: turns off heading colors.
- **`XDG_STATE_HOME`**: where tome keeps its state. Positions and preferences are stored in `$XDG_STATE_HOME/tome/`, which defaults to `~/.local/state/tome/`.

## Development

```sh
go test ./...
go run ./cmd/tome book.epub
scripts/build.sh          # release archives for every platform in dist/
```

To publish a release, push a tag such as `v0.1.0`. [GitHub Actions](.github/workflows/release.yml) then builds every platform and publishes the release. Design notes are in [docs/design.md](docs/design.md).

## License

[MIT](LICENSE)
