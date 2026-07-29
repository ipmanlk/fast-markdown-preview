package main

// htmlDocumentTemplate is the standalone HTML document shell. The rendered
// markdown body is injected into <main id="preview">, which is the node the
// browser diffs with morphdom. CSS is inlined so the page is fully
// self-contained (no extra HTTP round trips, no external resources).
const htmlDocumentTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Markdown Preview</title>
<style>
{{ .BaseStyles }}
{{ .HighlightCSS }}
{{ .DarkStyles }}
</style>
</head>
<body>
<main id="preview" class="markdown-body">
{{ .Content }}
</main>
</body>
</html>`

// baseStyles holds the light-theme typography and layout rules. The selectors
// are intentionally plain (tag + .markdown-body scope) to keep matching cheap.
const baseStyles = `
:root {
  --bg: #ffffff;
  --fg: #24292e;
  --muted: #656d76;
  --border: #d0d7de;
  --code-bg: #f6f8fa;
  --code-inline-bg: rgba(175,184,193,0.2);
  --link: #0969da;
  --quote-border: #d0d7de;
}
*, *::before, *::after { box-sizing: border-box; }
html, body { margin: 0; padding: 0; }
body {
  background: var(--bg);
  color: var(--fg);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  font-size: 16px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}
.markdown-body {
  max-width: 860px;
  margin: 0 auto;
  padding: 2rem 1.25rem 6rem;
  word-wrap: break-word;
}
.markdown-body > *:first-child { margin-top: 0; }
.markdown-body h1, .markdown-body h2, .markdown-body h3,
.markdown-body h4, .markdown-body h5, .markdown-body h6 {
  margin: 1.5em 0 0.6em;
  font-weight: 600;
  line-height: 1.25;
}
.markdown-body h1 { font-size: 2em; padding-bottom: .3em; border-bottom: 1px solid var(--border); }
.markdown-body h2 { font-size: 1.5em; padding-bottom: .3em; border-bottom: 1px solid var(--border); }
.markdown-body h3 { font-size: 1.25em; }
.markdown-body h4 { font-size: 1em; }
.markdown-body h5 { font-size: .9em; }
.markdown-body h6 { font-size: .85em; color: var(--muted); }
.markdown-body p { margin: 0 0 1em; }
.markdown-body a { color: var(--link); text-decoration: none; }
.markdown-body a:hover { text-decoration: underline; }
.markdown-body ul, .markdown-body ol { margin: 0 0 1em; padding-left: 2em; }
.markdown-body li { margin: .25em 0; }
.markdown-body li > ul, .markdown-body li > ol { margin: .25em 0; }
.markdown-body img { max-width: 100%; height: auto; }
.markdown-body hr { border: 0; border-top: 2px solid var(--border); margin: 1.5em 0; }
.markdown-body blockquote {
  margin: 0 0 1em;
  padding: 0 1em;
  color: var(--muted);
  border-left: .25em solid var(--quote-border);
}
.markdown-body code {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
  font-size: .875em;
  background: var(--code-inline-bg);
  border-radius: 4px;
  padding: .15em .35em;
}
.markdown-body pre {
  margin: 0 0 1em;
  padding: 1em;
  background: var(--code-bg);
  border-radius: 8px;
  overflow-x: auto;
  line-height: 1.45;
}
.markdown-body pre code {
  background: none;
  padding: 0;
  font-size: .85em;
}
.markdown-body table {
  border-collapse: collapse;
  display: block;
  width: max-content;
  max-width: 100%;
  overflow-x: auto;
  margin: 0 0 1em;
}
.markdown-body th, .markdown-body td {
  border: 1px solid var(--border);
  padding: 6px 13px;
}
.markdown-body th { background: var(--code-bg); font-weight: 600; }
.markdown-body tr:nth-child(2n) td { background: var(--code-bg); }
.markdown-body input[type="checkbox"] { margin-right: .4em; }
.markdown-body dl { margin: 0 0 1em; }
.markdown-body dt { font-weight: 600; margin-top: .5em; }
.markdown-body dd { margin: 0 0 .5em 1.5em; }
.markdown-body .footnote-definition { margin: 1em 0; font-size: .9em; color: var(--muted); }
.markdown-body .footnote-definition p { display: inline; margin: 0 0 0 .25em; }
.markdown-body kbd {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: .8em;
  background: var(--code-bg);
  border: 1px solid var(--border);
  border-bottom-width: 2px;
  border-radius: 6px;
  padding: .1em .4em;
}
.markdown-body mark { background: rgba(255, 212, 0, 0.4); padding: .1em .2em; border-radius: 3px; }
`

// darkStyles overrides the light theme when the page requests dark mode.
// The browser UI's setTheme() always sets data-theme="dark" or "light" on
// <html> (even in auto mode, it reads prefers-color-scheme and sets the
// attribute), so we only need explicit [data-theme="dark"] selectors. This
// avoids duplicating every chroma rule under a separate @media block.
const darkStyles = `
html[data-theme="dark"] {
  --bg: #0d1117;
  --fg: #c9d1d9;
  --muted: #8b949e;
  --border: #30363d;
  --code-bg: #161b22;
  --code-inline-bg: rgba(240,246,252,0.15);
  --link: #58a6ff;
  --quote-border: #30363d;
}
/* Chroma dark theme: re-map the light class names to dark colors. */
html[data-theme="dark"] .chroma { color: #c9d1d9; background-color: #161b22; }
html[data-theme="dark"] .chroma .err { color: #f85149; }
html[data-theme="dark"] .chroma .lnlinks { color: inherit; text-decoration: none; }
html[data-theme="dark"] .chroma .lnt { color: #6e7681; }
html[data-theme="dark"] .chroma .ln { color: #6e7681; }
html[data-theme="dark"] .chroma .k { color: #ff7b72; }
html[data-theme="dark"] .chroma .kc { color: #79c0ff; }
html[data-theme="dark"] .chroma .kd { color: #ff7b72; }
html[data-theme="dark"] .chroma .kn { color: #ff7b72; }
html[data-theme="dark"] .chroma .kp { color: #79c0ff; }
html[data-theme="dark"] .chroma .kr { color: #ff7b72; }
html[data-theme="dark"] .chroma .kt { color: #ff7b72; }
html[data-theme="dark"] .chroma .na { color: #79c0ff; }
html[data-theme="dark"] .chroma .nb { color: #c9d1d9; }
html[data-theme="dark"] .chroma .bp { color: #c9d1d9; }
html[data-theme="dark"] .chroma .nc { color: #f0883e; }
html[data-theme="dark"] .chroma .no { color: #79c0ff; }
html[data-theme="dark"] .chroma .nd { color: #d2a8ff; }
html[data-theme="dark"] .chroma .ni { color: #ffa657; }
html[data-theme="dark"] .chroma .ne { color: #f0883e; }
html[data-theme="dark"] .chroma .nf { color: #d2a8ff; }
html[data-theme="dark"] .chroma .s { color: #a5d6ff; }
html[data-theme="dark"] .chroma .sr { color: #79c0ff; }
html[data-theme="dark"] .chroma .m { color: #79c0ff; }
html[data-theme="dark"] .chroma .mb { color: #79c0ff; }
html[data-theme="dark"] .chroma .mf { color: #79c0ff; }
html[data-theme="dark"] .chroma .mh { color: #79c0ff; }
html[data-theme="dark"] .chroma .mi { color: #79c0ff; }
html[data-theme="dark"] .chroma .il { color: #79c0ff; }
html[data-theme="dark"] .chroma .o { color: #ff7b72; }
html[data-theme="dark"] .chroma .ow { color: #ff7b72; }
html[data-theme="dark"] .chroma .c { color: #8b949e; }
html[data-theme="dark"] .chroma .ch { color: #8b949e; }
html[data-theme="dark"] .chroma .cm { color: #8b949e; }
html[data-theme="dark"] .chroma .cpf { color: #8b949e; }
html[data-theme="dark"] .chroma .c1 { color: #8b949e; }
html[data-theme="dark"] .chroma .cs { color: #8b949e; }
html[data-theme="dark"] .chroma .cp { color: #8b949e; }
html[data-theme="dark"] .chroma .nt { color: #7ee787; }
html[data-theme="dark"] .chroma .nv { color: #79c0ff; }
html[data-theme="dark"] .chroma .vc { color: #79c0ff; }
html[data-theme="dark"] .chroma .vg { color: #79c0ff; }
html[data-theme="dark"] .chroma .vi { color: #79c0ff; }
html[data-theme="dark"] .chroma .gd { color: #ffa198; background-color: #490202; }
html[data-theme="dark"] .chroma .ge { font-style: italic; }
html[data-theme="dark"] .chroma .gi { color: #aff5b4; background-color: #033a16; }
html[data-theme="dark"] .chroma .gr { color: #ffa198; }
html[data-theme="dark"] .chroma .gh { color: #8b949e; }
html[data-theme="dark"] .chroma .gu { color: #8b949e; }
/* String literal variants: all map to the dark string color. Without these,
the light theme's #dd1144 (pink) would bleed through on dark backgrounds. */
html[data-theme="dark"] .chroma .dl { color: #79c0ff; }
html[data-theme="dark"] .chroma .s1 { color: #a5d6ff; }
html[data-theme="dark"] .chroma .s2 { color: #a5d6ff; }
html[data-theme="dark"] .chroma .sa { color: #79c0ff; }
html[data-theme="dark"] .chroma .sb { color: #a5d6ff; }
html[data-theme="dark"] .chroma .sc { color: #a5d6ff; }
html[data-theme="dark"] .chroma .sd { color: #a5d6ff; }
html[data-theme="dark"] .chroma .se { color: #79c0ff; }
html[data-theme="dark"] .chroma .sh { color: #79c0ff; }
html[data-theme="dark"] .chroma .si { color: #a5d6ff; }
html[data-theme="dark"] .chroma .ss { color: #a5d6ff; }
html[data-theme="dark"] .chroma .sx { color: #a5d6ff; }
/* Remaining tokens with light colors that need dark equivalents. */
html[data-theme="dark"] .chroma .go { color: #8b949e; }
html[data-theme="dark"] .chroma .gp { color: #8b949e; }
html[data-theme="dark"] .chroma .gt { color: #ff7b72; }
html[data-theme="dark"] .chroma .hl { background-color: #6e7681; }
html[data-theme="dark"] .chroma .mo { color: #a5d6ff; }
html[data-theme="dark"] .chroma .nl { color: #79c0ff; font-weight: bold; }
html[data-theme="dark"] .chroma .nn { color: #ff7b72; }
html[data-theme="dark"] .chroma .w { color: #6e7681; }
`
