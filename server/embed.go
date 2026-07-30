package main

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed frontend/index.html
var indexHTMLRaw string

//go:embed frontend/morphdom.min.js
var morphdomJS string

// indexTemplate is parsed once at startup. It is executed once in main() (after
// the markdown renderer is built) to produce the final, self-contained index
// page with the morphdom source and all stylesheets inlined. The result is
// stored on the Handler so the hot path (GET /) is a single string copy with no
// per-request work. The morphdom source is template data, so any braces it
// contains are inserted verbatim and never re-parsed as actions.
var indexTemplate *template.Template

func init() {
	indexTemplate = template.Must(template.New("index").Parse(indexHTMLRaw))
}

// renderIndex executes the index template with the given stylesheets. Called
// once at startup; the returned string is served unchanged by GET /.
func renderIndex(baseStyles, highlightCSS, darkStyles string) string {
	var buf bytes.Buffer
	if err := indexTemplate.Execute(&buf, map[string]string{
		"Morphdom":     morphdomJS,
		"BaseStyles":   baseStyles,
		"HighlightCSS": highlightCSS,
		"DarkStyles":   darkStyles,
	}); err != nil {
		panic("execute index template: " + err.Error())
	}
	return buf.String()
}
