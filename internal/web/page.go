package web

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/huyphung1602/tome/internal/epub"
)

const defaultReadHeight = 36

// buildPage renders the reader page: a frame like tome's (button bar, a
// contents sidebar, a reading column of limited width and height, a status
// line) around the whole book as one scrolling column. The book sits in a
// shadow root so its CSS can't break the page around it.
func buildPage(book *epub.Book, base string, theme Theme, readHeight int) []byte {
	body, inline := bookHTML(book, base)

	var sheets []string
	for i := range book.Styles {
		sheets = append(sheets, fmt.Sprintf("%scss/%d", base, i))
	}

	index := map[string]int{}
	for i, ch := range book.Chapters {
		index[ch.Path] = i
	}
	var toc strings.Builder
	for _, e := range book.TOC {
		i, ok := index[e.Path]
		if !ok {
			continue
		}
		target := chapterID(i)
		if e.Fragment != "" {
			target = anchorID(i, e.Fragment)
		}
		fmt.Fprintf(&toc, `<a href="#%s" data-target="%s" style="--depth:%d">%s</a>`+"\n",
			target, target, min(e.Depth, 4), html.EscapeString(e.Title))
	}
	if toc.Len() == 0 {
		for i := range book.Chapters {
			fmt.Fprintf(&toc, `<a href="#%s" data-target="%s" style="--depth:0">Chapter %d</a>`+"\n",
				chapterID(i), chapterID(i), i+1)
		}
	}

	if readHeight <= 0 {
		readHeight = defaultReadHeight
	}
	data, _ := json.Marshal(map[string]any{
		"html": body, "sheets": sheets, "inline": inline, "base": base,
		"lines": readHeight, "terminal": terminalTheme(theme),
	})
	title := book.Title
	if title == "" {
		title = "tome"
	}
	r := strings.NewReplacer(
		"{{FONTS}}", fontFaces(base),
		"{{TITLE}}", html.EscapeString(title),
		"{{TOC}}", toc.String(),
		"{{DATA}}", strings.ReplaceAll(string(data), "</", `<\/`),
	)
	return []byte(r.Replace(pageTemplate))
}

// terminalTheme turns the terminal's colors into the page's color
// variables, using the same ANSI colors tome uses: headings 5/4/6, keywords
// 4, strings 2, numbers 6, types 5, bookmarks 1. nil when unknown.
func terminalTheme(t Theme) map[string]any {
	if t.FG == "" || t.BG == "" {
		return nil
	}
	pal := func(n int, fallback string) string {
		if c := t.Palette[n]; c != "" {
			return c
		}
		return fallback
	}
	return map[string]any{
		"bg": t.BG, "fg": t.FG,
		"muted": blend(t.FG, t.BG, 0.5), "line": blend(t.FG, t.BG, 0.18),
		"h1": pal(5, t.FG), "h2": pal(4, t.FG), "h3": pal(6, t.FG), "accent": pal(4, t.FG),
		"kw": pal(4, t.FG), "str": pal(2, t.FG), "num": pal(6, t.FG), "type": pal(5, t.FG), "mark": pal(1, t.FG),
		"dark": luminance(t.BG) < 0.5,
	}
}

func rgb(hex string) (r, g, b float64) {
	if len(hex) != 7 {
		return
	}
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)
}

// blend mixes a into b by t (0 = b, 1 = a).
func blend(a, b string, t float64) string {
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	mix := func(x, y float64) int { return int(y + (x-y)*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

func luminance(hex string) float64 {
	r, g, b := rgb(hex)
	return (0.2126*r + 0.7152*g + 0.0722*b) / 255
}

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{TITLE}}</title>
<style>
{{FONTS}}
:root { color-scheme: light dark; --font: Georgia, "Times New Roman", serif; --size: 19px; --measure: 680px; }
* { box-sizing: border-box; }
html, body { margin: 0; height: 100%; background: var(--bg); color: var(--fg); overflow: hidden; }
/* grayscale antialiasing: lighter, crisper strokes than subpixel on macOS */
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; text-rendering: optimizeLegibility; }
body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; }
button { font: inherit; color: inherit; background: none; border: 0; padding: 0; cursor: pointer; }

/* the frame: centered as one group, like tome */
#frame { position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); display: grid;
  grid-template-columns: auto var(--measure); grid-template-rows: auto auto auto; column-gap: 3rem; row-gap: 2rem; }
body.no-toc #frame { grid-template-columns: var(--measure); }
#bar, #status { grid-column: -2; display: flex; justify-content: space-between; align-items: baseline; color: var(--muted); }
#bar .title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
#bar .buttons { display: flex; gap: 1.5rem; white-space: nowrap; }
#bar .buttons button { display: inline-flex; align-items: baseline; gap: .45rem; }
#bar .buttons button:hover { color: var(--fg); }
/* the key, as a small keycap */
kbd { font: 11px/1 ui-monospace, "SF Mono", Menlo, Consolas, monospace; min-width: 1.6em; text-align: center; padding: .2em .4em;
  border: 1px solid var(--line); border-bottom-width: 2px; border-radius: 4px; color: var(--fg); }
#bar .buttons button:hover kbd { border-color: var(--muted); }
#toc { grid-row: 2; width: 16rem; overflow-y: auto; border-right: 1px solid var(--line); padding-right: 1.5rem; scrollbar-width: none; }
#toc::-webkit-scrollbar, #page::-webkit-scrollbar { display: none; }
body.no-toc #toc { display: none; }
#toc a { display: block; padding: .15rem 0 .15rem calc(var(--depth) * 1.1rem); color: var(--muted); text-decoration: none; font-size: 13px; line-height: 1.4; }
#toc a:hover { color: var(--fg); }
#toc a.current { color: var(--fg); font-weight: 600; }
#page { grid-row: 2; grid-column: -2; overflow-y: auto; padding-left: 2.5rem; margin-left: -2.5rem; /* room for bookmark ribbons */ scrollbar-width: none; }
#status { grid-row: 3; font-size: 13px; }

#find { display: none; gap: .4rem; align-items: baseline; }
body.finding #find { display: inline-flex; } body.finding #where { display: none; }
#q { font: inherit; color: var(--fg); background: transparent; border: 0; border-bottom: 1px solid var(--line); outline: none; width: 16rem; padding: 0 0 1px; }
#note { margin-left: 1rem; }
.popup { position: absolute; z-index: 5; display: none; background: var(--bg); border: 1px solid var(--line); border-radius: 8px;
  padding: 1rem 1.25rem; box-shadow: 0 8px 30px rgba(0,0,0,.18); }
.popup.open { display: block; }
.popup h2 { font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 0 0 .5rem; }
#marks { width: 30rem; max-height: 60vh; overflow-y: auto; }
#markList .row { display: grid; grid-template-columns: 1fr auto; gap: .75rem; padding: .35rem .5rem; border-radius: 6px; cursor: pointer; }
#markList .row:hover { background: color-mix(in srgb, var(--fg) 8%, transparent); }
#markList .ch { color: var(--muted); font-size: 12px; } #markList .snip { display: block; }
#markList .del { color: var(--muted); } #markList .del:hover { color: var(--mark); }
#marks .hint { color: var(--muted); font-size: 12px; margin: .75rem 0 0; }
#settings { position: absolute; z-index: 5; display: none; background: var(--bg); border: 1px solid var(--line); border-radius: 8px;
  padding: 1rem 1.25rem; box-shadow: 0 8px 30px rgba(0,0,0,.18); min-width: 15rem; }
#settings.open { display: block; }
#settings h2 { font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 0 0 .5rem; }
#settings .row { display: flex; gap: .5rem; margin-bottom: 1rem; }
#settings .row:last-child { margin-bottom: 0; }
#settings button { border: 1px solid var(--line); border-radius: 6px; padding: .3rem .7rem; }
#settings button.on { border-color: var(--fg); font-weight: 600; }
@media (max-width: 900px) {
  #frame { grid-template-columns: min(var(--measure), 92vw); }
  #toc { position: fixed; inset: 0 auto 0 0; width: 80vw; max-width: 20rem; background: var(--bg); z-index: 4; padding: 2rem 1.5rem; display: none; }
  body.toc-open #toc { display: block; }
}
</style>
</head>
<body>
<div id="frame">
  <div id="bar"><span class="title">{{TITLE}}</span><span class="buttons">
    <button id="tocBtn" title="contents (t)"><kbd>t</kbd>contents</button>
    <button id="findBtn" title="search (/)"><kbd>/</kbd>search</button>
    <button id="marksBtn" title="bookmarks (')"><kbd>'</kbd>bookmarks</button>
    <button id="gear" title="theme and font"><kbd>,</kbd>settings</button></span></div>
  <nav id="toc">{{TOC}}</nav>
  <div id="page"><div id="book"></div></div>
  <div id="status"><span><span id="where"></span><span id="find"><span>/</span><input id="q" autocomplete="off" spellcheck="false"><span id="count"></span></span><span id="note"></span></span><span id="sync"></span></div>
</div>
<div id="marks" class="popup"><h2>bookmarks</h2><div id="markList"></div>
  <p class="hint">B bookmarks the paragraph at the top · or click the left margin next to a paragraph</p></div>
<div id="settings">
  <h2>theme</h2><div class="row" data-key="theme"><button data-v="terminal">terminal</button><button data-v="paper">paper</button><button data-v="night">night</button></div>
  <h2>font</h2><div class="row" data-key="font"><button data-v="mono">mono</button><button data-v="serif">serif</button><button data-v="sans">sans</button></div>
</div>
<script>
const data = {{DATA}};
const THEMES = {
  paper: { bg: "#f6f1e7", fg: "#2e2a24", muted: "#8a8170", line: "#e3dccd", h1: "#7d3c6b", h2: "#2f5d8a", h3: "#2f7a7a", accent: "#2f5d8a",
           kw: "#2f5d8a", str: "#4f7a2a", num: "#2f7a7a", type: "#7d3c6b", mark: "#b5442f", dark: false },
  night: { bg: "#1d1c1a", fg: "#d6cebd", muted: "#8a8373", line: "#34322d", h1: "#c792b8", h2: "#8fb3d9", h3: "#86c1b9", accent: "#8fb3d9",
           kw: "#8fb3d9", str: "#a9c98a", num: "#86c1b9", type: "#c792b8", mark: "#e07a64", dark: true },
};
const FONTS = {
  mono:  { font: '"SF Mono", Menlo, Monaco, "Cascadia Mono", Consolas, monospace', size: 16, measure: "72ch" },
  serif: { font: 'Literata, Georgia, "Times New Roman", serif', size: 18, measure: "680px" },
  sans:  { font: '"Be Vietnam Pro", system-ui, -apple-system, "Segoe UI", sans-serif', size: 17, measure: "680px" },
};
const store = { get(k, d) { try { return localStorage.getItem("tome." + k) || d; } catch { return d; } },
                set(k, v) { try { localStorage.setItem("tome." + k, v); } catch {} } };

const page = document.getElementById("page");
const host = document.getElementById("book");
const root = host.attachShadow({ mode: "open" });
const base = document.createElement("style");
base.textContent = ` + "`" + `
:host { display: block; font-family: var(--font); font-size: var(--size); line-height: 1.7; color: var(--fg); }
img, svg, video { max-width: 100%; height: auto; max-height: calc(var(--frame-h) - 2rem); object-fit: contain; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: .85em; }
table { max-width: 100%; display: block; overflow-x: auto; }
.tome-chapter { padding-bottom: 4rem; }
[id] { scroll-margin-top: 1rem; }
::highlight(tome-hit) { background: color-mix(in srgb, var(--accent) 22%, transparent); }
::highlight(tome-cur) { background: var(--accent); color: var(--bg); }
.tome-bm, .tome-hover { position: relative; }
/* a ribbon in the margin, at the first line */
:is(.tome-bm, .tome-hover)::before { content: ""; position: absolute; left: calc(-1.5rem - var(--bm-x, 0px)); top: calc((1lh - 14px) / 2);
  width: 8px; height: 14px; background: var(--mark); clip-path: polygon(0 0, 100% 0, 100% 100%, 50% 72%, 0 100%); transition: opacity .15s; }
/* the paragraph under the mouse shows a faint one where a click bookmarks it */
.tome-hover:not(.tome-bm)::before { background: var(--muted); opacity: .3; }
.tome-hover.tome-edge:not(.tome-bm)::before { background: var(--mark); opacity: .75; }
.tome-bm.tome-edge::before { opacity: .35; } /* fading: a click removes it */
` + "`" + `;
root.appendChild(base);
for (const href of data.sheets) { const l = document.createElement("link"); l.rel = "stylesheet"; l.href = href; root.appendChild(l); }
for (const css of data.inline) { const s = document.createElement("style"); s.textContent = css; root.appendChild(s); }
// after the book's CSS: the reading font and theme colors win, like in tome
const theme = document.createElement("style");
theme.textContent = ` + "`" + `
/* :where() keeps this at :host specificity so the colors below win */
:host :where(*:not(img, svg, svg *)) { color: inherit !important; background-color: transparent !important; border-color: var(--line) !important; }
:host :where(p, li, dd, blockquote, div, span, td, th) { font-family: var(--font) !important; }
:host h1 { color: var(--h1) !important; } :host h2 { color: var(--h2) !important; } :host :is(h3, h4, h5, h6) { color: var(--h3) !important; }
:host a { color: var(--accent) !important; }
:host :is(code, kbd, samp, tt) { color: var(--str) !important; font-family: "SF Mono", Menlo, monospace !important; }
:host :is(pre, pre *) { font-family: "SF Mono", Menlo, monospace !important; }
:host :is(pre code, pre code *) { color: inherit !important; }
:host :is(.k, .kd, .kn, .kr, .kp, .kc, .kv) { color: var(--kw) !important; } :host :is(.kt, .nc, .nb) { color: var(--type) !important; }
:host :is(.s, .s1, .s2, .sb, .sc, .sd, .se, .sh, .si, .sx, .sr, .ss) { color: var(--str) !important; }
:host :is(.m, .mi, .mf, .mh, .mo, .il) { color: var(--num) !important; }
:host :is(.c, .c1, .cm, .cs, .ch, .cp) { color: var(--muted) !important; font-style: italic; }
:host(.dark) img { background: #fff; }
` + "`" + `;
const content = document.createElement("div");
content.innerHTML = data.html;
root.appendChild(content);
root.appendChild(theme);

// ---- theme and font
function apply() {
  const tname = store.get("theme", "terminal");
  const t = tname === "terminal" ? (data.terminal || (matchMedia("(prefers-color-scheme: dark)").matches ? THEMES.night : THEMES.paper)) : THEMES[tname];
  for (const [k, v] of Object.entries(t)) if (k !== "dark") document.documentElement.style.setProperty("--" + k, v);
  host.classList.toggle("dark", !!t.dark);
  const f = FONTS[store.get("font", "serif")];
  document.documentElement.style.setProperty("--font", f.font);
  document.documentElement.style.setProperty("--size", f.size + "px");
  document.documentElement.style.setProperty("--measure", f.measure);
  for (const row of document.querySelectorAll("#settings .row"))
    for (const b of row.children) b.classList.toggle("on", b.dataset.v === store.get(row.dataset.key, row.dataset.key === "theme" ? "terminal" : "serif"));
  layout();
  placeMarks();
}
// the reading frame: at most data.lines lines tall, like tome's
let lineH = 32;
function layout() {
  const size = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--size")) || 19;
  lineH = size * 1.7;
  const room = innerHeight - 190;
  const lines = Math.max(8, Math.min(data.lines, Math.floor(room / lineH)));
  const h = Math.round(lines * lineH);
  page.style.height = h + "px";
  document.getElementById("toc").style.height = h + "px";
  document.documentElement.style.setProperty("--frame-h", h + "px");
}
const gear = document.getElementById("gear"), settings = document.getElementById("settings");
gear.addEventListener("click", e => {
  e.stopPropagation();
  const r = gear.getBoundingClientRect();
  settings.style.top = (r.bottom + 8) + "px";
  settings.style.right = (innerWidth - r.right) + "px";
  settings.classList.toggle("open");
});
settings.addEventListener("click", e => {
  e.stopPropagation();
  const b = e.target.closest("button"); if (!b) return;
  store.set(b.parentElement.dataset.key, b.dataset.v); apply();
});
document.addEventListener("click", () => settings.classList.remove("open"));
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", apply);
addEventListener("resize", layout);

// ---- contents
function toggleTOC() {
  if (innerWidth <= 900) document.body.classList.toggle("toc-open");
  else document.body.classList.toggle("no-toc");
  store.set("toc", document.body.classList.contains("no-toc") ? "hidden" : "shown");
}
if (store.get("toc", "shown") === "hidden") document.body.classList.add("no-toc");
document.getElementById("tocBtn").addEventListener("click", toggleTOC);
const links = [...document.querySelectorAll("#toc a")];
const targets = links.map(a => root.getElementById(a.dataset.target));
document.getElementById("toc").addEventListener("click", e => {
  const a = e.target.closest("a"); if (!a) return;
  e.preventDefault(); go(a.dataset.target); document.body.classList.remove("toc-open");
});
root.addEventListener("click", e => {
  const a = e.target.closest("a[href^='#']"); if (!a) return;
  e.preventDefault(); go(a.getAttribute("href").slice(1));
});
function go(id) { const el = root.getElementById(id); if (el) page.scrollTop += el.getBoundingClientRect().top - page.getBoundingClientRect().top - 8; }

// ---- status: chapter and progress, and the contents entry being read
function status() {
  const top = page.getBoundingClientRect().top + 24;
  let cur = -1;
  for (let i = 0; i < targets.length; i++) { const el = targets[i]; if (el && el.getBoundingClientRect().top <= top) cur = i; }
  links.forEach((a, i) => a.classList.toggle("current", i === cur));
  if (cur >= 0) {
    const a = links[cur], box = document.getElementById("toc");
    if (a.offsetTop < box.scrollTop || a.offsetTop > box.scrollTop + box.clientHeight - 40) box.scrollTop = a.offsetTop - box.clientHeight / 3;
  }
  const max = page.scrollHeight - page.clientHeight;
  const pct = max > 0 ? Math.round(100 * page.scrollTop / max) : 100;
  document.getElementById("where").textContent = (cur >= 0 ? links[cur].textContent + " · " : "") + pct + "%";
}

// ---- keys, like tome: pages turn by the whole frame
addEventListener("keydown", e => {
  if (e.metaKey || e.ctrlKey || e.altKey) return;
  const step = { " ": 1, PageDown: 1, ArrowRight: 1, l: 1, b: -1, PageUp: -1, ArrowLeft: -1, h: -1 }[e.key];
  if (step) { e.preventDefault(); page.scrollTop += step * (page.clientHeight - (page.clientHeight % lineH)); return; }
  const line = { j: 1, ArrowDown: 1, k: -1, ArrowUp: -1 }[e.key];
  if (line) { e.preventDefault(); page.scrollTop += line * lineH; return; }
  if (e.key === "g" || e.key === "Home") { e.preventDefault(); page.scrollTop = 0; }
  if (e.key === "G" || e.key === "End") { e.preventDefault(); page.scrollTop = page.scrollHeight; }
  if (e.key === "t") toggleTOC();
  if (e.key === "Escape") { settings.classList.remove("open"); document.body.classList.remove("toc-open"); }
});

// ---- sync with tome: positions are (chapter, block), the same unit tome
// saves; elements that start a block carry data-tb
const sections = [...root.querySelectorAll("section.tome-chapter")];
// blockAt is the block at the top of the frame: the last element, in
// document order, that starts at or above that line. Containers start
// before their children, so nested elements resolve to the innermost one.
function blockAt() {
  const top = page.getBoundingClientRect().top + 8;
  for (const sec of sections) {
    if (sec.getBoundingClientRect().bottom <= top) continue;
    let best = null;
    for (const el of sec.querySelectorAll("[data-tb]")) {
      if (el.getBoundingClientRect().top > top) break;
      best = el;
    }
    return { c: +sec.id.slice(1), b: best ? +best.dataset.tb : 0 };
  }
  return null;
}
function showBlock(c, b) {
  const sec = root.getElementById("c" + c); if (!sec) return;
  let target = sec;
  for (const el of sec.querySelectorAll("[data-tb]")) { if (+el.dataset.tb <= b) target = el; else break; }
  page.scrollTop += target.getBoundingClientRect().top - page.getBoundingClientRect().top;
}
let quietUntil = 0, sent = "", timer = 0;
page.addEventListener("scroll", () => {
  requestAnimationFrame(status);
  if (Date.now() < quietUntil) return; // our own jump, don't echo it back
  clearTimeout(timer);
  timer = setTimeout(() => {
    const p = blockAt(); if (!p) return;
    const key = p.c + "," + p.b; if (key === sent) return;
    sent = key;
    fetch(data.base + "pos?c=" + p.c + "&b=" + p.b).catch(() => {});
  }, 120);
}, { passive: true });
function jump(c, b) { quietUntil = Date.now() + 400; sent = c + "," + b; showBlock(c, b); }
function connect() {
  const es = new EventSource(data.base + "events");
  const label = document.getElementById("sync");
  es.addEventListener("open", () => { label.textContent = "synced with tome"; });
  es.addEventListener("pos", e => { const [c, b] = e.data.split(",").map(Number); jump(c, b); });
  es.addEventListener("book", () => location.reload());
  es.addEventListener("bookmarks", e => {
    const before = marks ? marks.length : null;
    marks = JSON.parse(e.data);
    renderMarks();
    if (before !== null && before !== marks.length) note(marks.length > before ? "bookmarked" : "bookmark removed");
  });
  es.addEventListener("error", () => { label.textContent = "tome closed"; });
}

// ---- status notes
let noteTimer = 0;
function note(msg) {
  const el = document.getElementById("note"); el.textContent = msg;
  clearTimeout(noteTimer); noteTimer = setTimeout(() => { el.textContent = ""; }, 2500);
}

// ---- bookmarks, shared with tome (tome stores them; the page shows and toggles)
let marks = null;
function elementFor(c, b) {
  const sec = root.getElementById("c" + c); if (!sec) return null;
  let target = null;
  for (const el of sec.querySelectorAll("[data-tb]")) { if (+el.dataset.tb <= b) target = el; else break; }
  return target;
}
function chapterOf(el) {
  let title = "";
  for (let i = 0; i < targets.length; i++) {
    const t = targets[i];
    if (t && (t === el || t.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING)) title = links[i].textContent;
  }
  return title;
}
// ribbons sit in the margin left of the column, wherever the book indents
// the element itself
function placeMark(el) {
  const left = page.getBoundingClientRect().left + parseFloat(getComputedStyle(page).paddingLeft);
  el.style.setProperty("--bm-x", (el.getBoundingClientRect().left - left) + "px");
}
function placeMarks() { for (const el of root.querySelectorAll(".tome-bm")) placeMark(el); }
addEventListener("resize", placeMarks);

function renderMarks() {
  for (const el of root.querySelectorAll(".tome-bm")) el.classList.remove("tome-bm");
  for (const [c, b] of marks) { const el = elementFor(c, b); if (el) el.classList.add("tome-bm"); }
  placeMarks();
  const list = document.getElementById("markList");
  list.innerHTML = "";
  if (!marks.length) { list.innerHTML = '<p class="hint">no bookmarks yet</p>'; return; }
  marks.forEach(([c, b]) => {
    const el = elementFor(c, b);
    const row = document.createElement("div"); row.className = "row";
    const text = document.createElement("span");
    const ch = document.createElement("span"); ch.className = "ch"; ch.textContent = el ? chapterOf(el) : "";
    const sn = document.createElement("span"); sn.className = "snip";
    sn.textContent = el ? el.textContent.trim().replace(/\s+/g, " ").slice(0, 90) : c + ":" + b;
    text.append(ch, sn);
    const del = document.createElement("button"); del.className = "del"; del.textContent = "×"; del.title = "delete";
    del.addEventListener("click", e => { e.stopPropagation(); toggleMark(c, b); });
    row.append(text, del);
    row.addEventListener("click", () => { closePopups(); jump(c, b); report(); });
    list.append(row);
  });
}
function toggleMark(c, b) { fetch(data.base + "bookmark?c=" + c + "&b=" + b, { method: "POST" }).catch(() => note("tome isn't running")); }
function report() { setTimeout(() => { const p = blockAt(); if (p) fetch(data.base + "pos?c=" + p.c + "&b=" + p.b).catch(() => {}); }, 50); }
// a click in the left margin toggles the paragraph next to it, like tome
function marginBlock(e) {
  const box = host.getBoundingClientRect(), edge = e.clientX < box.left;
  const el = root.elementFromPoint(edge ? box.left + 12 : e.clientX, e.clientY);
  return { blk: el && el.closest("[data-tb]"), edge };
}
page.addEventListener("click", e => {
  const { blk, edge } = marginBlock(e); if (!edge || !blk) return;
  toggleMark(+blk.closest("section").id.slice(1), +blk.dataset.tb);
});
// hovering shows where that click goes: a faint ribbon beside the paragraph,
// stronger with a pointer once the mouse is in the margin
let hovered = null;
function hover(blk, edge) {
  if (hovered !== blk) {
    if (hovered) hovered.classList.remove("tome-hover", "tome-edge");
    hovered = blk;
    if (blk) { blk.classList.add("tome-hover"); placeMark(blk); }
  }
  if (blk) blk.classList.toggle("tome-edge", edge);
  const on = !!blk && edge;
  page.style.cursor = on ? "pointer" : "";
  page.title = on ? (blk.classList.contains("tome-bm") ? "remove bookmark" : "bookmark this paragraph (B)") : "";
}
page.addEventListener("mousemove", e => { const { blk, edge } = marginBlock(e); hover(blk, edge); });
page.addEventListener("mouseleave", () => hover(null, false));
function openPopup(id, button) {
  const pop = document.getElementById(id), r = document.getElementById(button).getBoundingClientRect();
  const wasOpen = pop.classList.contains("open");
  closePopups();
  if (wasOpen) return;
  pop.style.top = (r.bottom + 8) + "px"; pop.style.right = (innerWidth - r.right) + "px";
  pop.classList.add("open");
}
function closePopups() { for (const p of document.querySelectorAll(".popup, #settings")) p.classList.remove("open"); }
document.getElementById("marksBtn").addEventListener("click", e => { e.stopPropagation(); openPopup("marks", "marksBtn"); });
document.getElementById("marks").addEventListener("click", e => e.stopPropagation());
document.addEventListener("click", closePopups);

// ---- search, "smart accent" like tome: no accents typed = accents ignored
const hasHL = typeof Highlight !== "undefined" && CSS.highlights;
let hits = [], cur = -1, index = null;
function fold(s, keepMarks) {
  s = s.normalize("NFD").toLowerCase();
  if (!keepMarks) s = s.replace(/[̀-ͯ]/g, "").replace(/đ/g, "d");
  return s;
}
// one string for the whole book, each character mapped back to its text node
function buildIndex(keepMarks) {
  const chars = [], nodes = [], offs = [];
  const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT);
  let prevBlock = null, lastSpace = true;
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const blk = n.parentElement && n.parentElement.closest("p, li, h1, h2, h3, h4, h5, h6, pre, td, th, div, blockquote, figcaption, dt, dd");
    if (blk !== prevBlock && !lastSpace) { chars.push(" "); nodes.push(n); offs.push(0); lastSpace = true; }
    prevBlock = blk;
    const t = n.data;
    for (let i = 0; i < t.length; i++) {
      if (/\s/.test(t[i])) { if (!lastSpace) { chars.push(" "); nodes.push(n); offs.push(i); lastSpace = true; } continue; }
      for (const f of fold(t[i], keepMarks)) { chars.push(f); nodes.push(n); offs.push(i); }
      lastSpace = false;
    }
  }
  return { text: chars.join(""), nodes, offs, keep: keepMarks };
}
function search(text) {
  text = text.trim().replace(/\s+/g, " ");
  hits = []; cur = -1;
  if (text) {
    const keep = fold(text, true) !== fold(text, false);
    if (!index || index.keep !== keep) index = buildIndex(keep);
    const needle = fold(text, keep);
    for (let at = index.text.indexOf(needle); at >= 0 && hits.length < 3000; at = index.text.indexOf(needle, at + needle.length)) {
      const end = at + needle.length - 1, r = document.createRange();
      r.setStart(index.nodes[at], index.offs[at]);
      r.setEnd(index.nodes[end], Math.min(index.nodes[end].data.length, index.offs[end] + 1));
      hits.push(r);
    }
  }
  if (hasHL) CSS.highlights.set("tome-hit", new Highlight(...hits));
  // start from the first hit at or after the top of the frame
  const top = page.getBoundingClientRect().top;
  cur = hits.findIndex(r => r.getBoundingClientRect().top >= top - 2);
  if (cur < 0 && hits.length) cur = 0;
  showHit();
}
function showHit() {
  const count = document.getElementById("count");
  if (!hits.length) { count.textContent = q.value.trim() ? "no matches" : ""; if (hasHL) CSS.highlights.delete("tome-cur"); return; }
  count.textContent = (cur + 1) + "/" + hits.length;
  const r = hits[cur];
  if (hasHL) CSS.highlights.set("tome-cur", new Highlight(r));
  const rect = r.getBoundingClientRect(), box = page.getBoundingClientRect();
  if (rect.top < box.top || rect.bottom > box.bottom) page.scrollTop += rect.top - box.top - box.height / 3;
}
function step(d) { if (!hits.length) return; cur = (cur + d + hits.length) % hits.length; showHit(); }
const q = document.getElementById("q");
function openFind() { document.body.classList.add("finding"); q.focus(); q.select(); }
function closeFind() {
  document.body.classList.remove("finding"); q.value = ""; hits = []; cur = -1; q.blur();
  document.getElementById("count").textContent = "";
  if (hasHL) { CSS.highlights.delete("tome-hit"); CSS.highlights.delete("tome-cur"); }
}
let typing = 0;
q.addEventListener("input", () => { clearTimeout(typing); typing = setTimeout(() => search(q.value), 150); });
q.addEventListener("keydown", e => {
  e.stopPropagation();
  if (e.key === "Enter") { e.preventDefault(); if (!hits.length) search(q.value); else step(e.shiftKey ? -1 : 1); }
  if (e.key === "Escape") { e.preventDefault(); closeFind(); }
});
document.getElementById("findBtn").addEventListener("click", e => { e.stopPropagation(); openFind(); });

// keys for search and bookmarks (the page keys above handle paging)
addEventListener("keydown", e => {
  if (e.metaKey || e.ctrlKey || e.altKey || e.target === q) return;
  if (e.key === "/") { e.preventDefault(); openFind(); }
  else if (e.key === "n" && hits.length) step(1);
  else if (e.key === "N" && hits.length) step(-1);
  else if (e.key === "B") { const p = blockAt(); if (p) toggleMark(p.c, p.b); }
  else if (e.key === "'") { e.preventDefault(); openPopup("marks", "marksBtn"); }
  else if (e.key === ",") gear.click();
  else if (e.key === "Escape") { closePopups(); if (hits.length || document.body.classList.contains("finding")) closeFind(); }
});

apply();
Promise.all([...root.querySelectorAll("link")].map(l => new Promise(r => { l.onload = l.onerror = r; }))).then(() => {
  const m = location.hash.match(/^#p(\d+)-(\d+)$/);
  if (m) jump(+m[1], +m[2]); else if (location.hash) go(location.hash.slice(1));
  status();
  connect();
});
</script>
</body>
</html>
`
