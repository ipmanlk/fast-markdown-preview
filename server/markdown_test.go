package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

// TestRenderBasicFeatures checks that the renderer produces the expected HTML
// for the core GFM feature set. These are smoke tests of the goldmark
// configuration, not full CommonMark conformance (goldmark is already
// fuzz-tested upstream).
func TestRenderBasicFeatures(t *testing.T) {
	r, err := NewMarkdownRenderer()
	if err != nil {
		t.Fatalf("NewMarkdownRenderer: %v", err)
	}

	cases := []struct {
		name     string
		md       string
		contains []string
	}{
		{
			name: "heading with auto id",
			md:   "# Hello World",
			contains: []string{
				`<h1 id="hello-world">Hello World</h1>`,
			},
		},
		{
			name: "bold and italic",
			md:   "**bold** and *italic*",
			contains: []string{
				"<strong>bold</strong>",
				"<em>italic</em>",
			},
		},
		{
			name: "inline code",
			md:   "Use `fmt.Println` to print.",
			contains: []string{
				"<code>fmt.Println</code>",
			},
		},
		{
			name: "gfm table",
			md:   "| a | b |\n|---|---|\n| 1 | 2 |",
			contains: []string{
				"<table>",
				"<th>a</th>",
				"<td>1</td>",
			},
		},
		{
			name: "strikethrough",
			md:   "~~deleted~~",
			contains: []string{
				"<del>deleted</del>",
			},
		},
		{
			name: "task list",
			md:   "- [x] done\n- [ ] todo",
			contains: []string{
				`type="checkbox"`,
				"checked",
				"todo",
			},
		},
		{
			name: "fenced code with highlighting",
			md:   "```go\nfunc main() {}\n```",
			contains: []string{
				`<pre class="chroma">`,
				`<code>`,
				`class="kd"`, // chroma keyword class
			},
		},
		{
			name: "footnote",
			md:   "Text[^1].\n\n[^1]: note body",
			contains: []string{
				`<a href="#fn:1"`,
				`footnote-definition`,
			},
		},
		{
			name: "definition list",
			md:   "Term\n: Definition",
			contains: []string{
				"<dl>",
				"<dt>Term</dt>",
				"<dd>Definition</dd>",
			},
		},
		{
			name: "emoji shortcode",
			md:   "Hello :smile:",
			contains: []string{
				"😄",
			},
		},
		{
			name: "autolink bare url",
			md:   "Visit https://example.com today",
			contains: []string{
				`href="https://example.com"`,
			},
		},
		{
			name: "blockquote",
			md:   "> quoted text",
			contains: []string{
				"<blockquote>",
				"quoted text",
			},
		},
		{
			name: "typographer smart quotes",
			md:   `"quoted" and -- dash`,
			contains: []string{
				"&ldquo;", // smart left double quote
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := r.Render([]byte(tc.md))
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\noutput:\n%s", want, out)
				}
			}
		})
	}
}

// TestRenderWrapsInDocument verifies the output is a complete HTML document
// with the preview container, so morphdom has a stable target to diff.
func TestRenderWrapsInDocument(t *testing.T) {
	r, err := NewMarkdownRenderer()
	if err != nil {
		t.Fatalf("NewMarkdownRenderer: %v", err)
	}

	out, err := r.Render([]byte("# Title"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	for _, want := range []string{
		"<!DOCTYPE html>",
		`<main id="preview" class="markdown-body">`,
		`<h1 id="title">Title</h1>`,
		"</main>",
		"</html>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// TestRenderEmptyInput ensures empty markdown does not error and still
// produces a valid document shell.
func TestRenderEmptyInput(t *testing.T) {
	r, err := NewMarkdownRenderer()
	if err != nil {
		t.Fatalf("NewMarkdownRenderer: %v", err)
	}
	out, err := r.Render([]byte(""))
	if err != nil {
		t.Fatalf("Render(empty): %v", err)
	}
	if !strings.Contains(out, `<main id="preview"`) {
		t.Errorf("empty render missing preview container:\n%s", out)
	}
}

// TestStylesNoMalformedSelectors verifies dark-mode CSS selectors are
// syntactically valid, so syntax highlighting is not broken in dark mode.
func TestStylesNoMalformedSelectors(t *testing.T) {
	for _, css := range []string{baseStyles, darkStyles} {
		// Every ":not(" must be followed by "[" for a valid attribute selector.
		if strings.Contains(css, ":not(data-theme") {
			t.Errorf("malformed :not() selector (missing '[') in CSS")
		}
	}
}

// TestDarkChromaCoversAllColoredTokens ensures every chroma token class that
// carries a color in the light "github" style has a corresponding
// html[data-theme="dark"] override. Without it, light-theme colors bleed
// through in dark mode (e.g. strings render pink instead of blue). The
// structural classes (line, lntable, lntd, gl, gs) are layout-only and need
// no dark override.
func TestDarkChromaCoversAllColoredTokens(t *testing.T) {
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	style := styles.Get("github")
	if style == nil {
		t.Fatal("github style not found")
	}
	var cssBuf bytes.Buffer
	if err := formatter.WriteCSS(&cssBuf, style); err != nil {
		t.Fatalf("WriteCSS: %v", err)
	}

	// Collect every token class the light CSS emits, and whether it sets a
	// color or background-color (those are the ones that bleed in dark mode).
	classRe := regexp.MustCompile(`\.chroma \.([a-zA-Z0-9]+)\s*\{([^}]*)\}`)
	lightColored := map[string]bool{}
	for _, m := range classRe.FindAllStringSubmatch(cssBuf.String(), -1) {
		if strings.Contains(m[2], "color") {
			lightColored[m[1]] = true
		}
	}

	// Classes that are purely structural (no color) and need no dark override.
	structural := map[string]bool{
		"line": true, "lntable": true, "lntd": true,
		"gl": true, // text-decoration: underline only
		"gs": true, // font-weight: bold only
	}

	for class := range lightColored {
		if structural[class] {
			continue
		}
		selector := `html[data-theme="dark"] .chroma .` + class
		if !strings.Contains(darkStyles, selector) {
			t.Errorf("dark CSS missing override for colored token .%q (would bleed light color in dark mode)", class)
		}
	}
}

// TestSplitSSELines verifies the SSE multi-line data encoding. A payload with
// newlines must be split into multiple "data:" lines so EventSource rejoins
// them with "\n" on the client.
func TestSplitSSELines(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"single", "single"},
		{"a\nb", "a\ndata:b"},
		{"a\nb\nc", "a\ndata:b\ndata:c"},
		{"", ""},
	}
	for _, c := range cases {
		got := splitSSELines(c.in)
		if got != c.want {
			t.Errorf("splitSSELines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
