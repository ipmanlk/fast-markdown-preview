package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	emojiast "github.com/yuin/goldmark-emoji/ast"
	highlight "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// MarkdownRenderer wraps a configured goldmark instance and renders markdown
// into a complete, standalone HTML document. The document is the unit that is
// diffed client-side by morphdom, so it must be self-contained (inline CSS,
// no external resources).
type MarkdownRenderer struct {
	md           goldmark.Markdown
	tpl          *template.Template
	highlightCSS string
}

// NewMarkdownRenderer builds a renderer with the full GFM-plus extension set:
// tables, strikethrough, task lists, definition lists, footnotes, typographer,
// autolinks, auto heading IDs, emoji, and chroma syntax highlighting.
func NewMarkdownRenderer() (*MarkdownRenderer, error) {
	// Generate the chroma stylesheet once at startup. Doing this per request
	// would re-allocate the formatter and walk every style entry on the hot
	// path, which is wasteful. We use the "github" light theme; the dark
	// overrides live in styles.go.
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	style := styles.Get("github")
	if style == nil {
		return nil, fmt.Errorf("chroma style %q not found", "github")
	}
	var cssBuf bytes.Buffer
	if err := formatter.WriteCSS(&cssBuf, style); err != nil {
		return nil, fmt.Errorf("generate highlight css: %w", err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,            // tables, strikethrough, task lists, autolinks, linkify
			extension.Footnote,       // [^1] footnotes
			extension.DefinitionList, // markdown extra definition lists
			extension.Typographer,    // smart quotes, dashes, ellipses
			emoji.New(
				emoji.WithRenderingMethod(emoji.Func),
				emoji.WithRendererFunc(unicodeEmojiRenderer),
			), // :smile: -> native unicode
			highlight.NewHighlighting(
				highlight.WithStyle("github"),
				highlight.WithFormatOptions(
					chromahtml.WithClasses(true), // emit class names; CSS provides colors
					chromahtml.TabWidth(4),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // <h2 id="..."> for anchor links
			parser.WithAttribute(),     // {.class} attribute syntax on blocks
		),
		goldmark.WithRendererOptions(
			goldmarkhtml.WithHardWraps(), // treat single newlines as <br>
			goldmarkhtml.WithUnsafe(),    // allow raw HTML passthrough (markdown is local/trusted)
		),
	)

	tpl := template.Must(template.New("doc").Parse(htmlDocumentTemplate))

	return &MarkdownRenderer{
		md:           md,
		tpl:          tpl,
		highlightCSS: cssBuf.String(),
	}, nil
}

// Render converts markdown source into a full HTML document string.
func (r *MarkdownRenderer) Render(src []byte) (string, error) {
	var body bytes.Buffer
	// goldmark.Convert reads from the byte slice without mutating it.
	if err := r.md.Convert(src, &body); err != nil {
		return "", fmt.Errorf("convert markdown: %w", err)
	}

	var out bytes.Buffer
	if err := r.tpl.Execute(&out, map[string]string{
		"Content":      body.String(),
		"BaseStyles":   baseStyles,
		"DarkStyles":   darkStyles,
		"HighlightCSS": r.highlightCSS,
	}); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return out.String(), nil
}

// splitSSELines prepares an HTML payload for an SSE "data:" field. SSE events
// are terminated by a blank line; a payload containing newlines must be split
// into multiple "data:" lines so the receiver can rejoin them losslessly.
// This is the standard EventSource wire format.
func splitSSELines(s string) string {
	// Replace every "\n" with "\ndata:" so each source line becomes its own
	// data: line. The caller writes "data: <result>\n\n".
	return strings.ReplaceAll(s, "\n", "\ndata:")
}

// unicodeEmojiRenderer renders :smile: shortcodes as native unicode emoji
// characters instead of Twemoji <img> tags. This keeps the page fully
// self-contained (no CDN requests) and is the lowest-cost rendering path.
func unicodeEmojiRenderer(w util.BufWriter, source []byte, n *emojiast.Emoji, config *emoji.RendererConfig) {
	if n.Value != nil && n.Value.IsUnicode() {
		for _, r := range n.Value.Unicode {
			w.WriteRune(r)
		}
		return
	}
	// Fallback: write the raw :shortname: text if no unicode mapping exists.
	w.Write(n.ShortName)
}
