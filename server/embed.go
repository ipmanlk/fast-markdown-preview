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

// indexHTML is the fully-assembled browser UI, with the morphdom source
// inlined into the script placeholder. Assembled once at init so the hot
// path (GET /) is a single string copy with no per-request work.
var indexHTML string

func init() {
	// index.html is a Go text/template. Its {{ .Morphdom }} placeholder
	// marks where the inlined morphdom source goes. We execute it once at
	// startup so the frontend stays a single self-contained page with zero
	// external requests. The morphdom source is template data, so any braces
	// it contains are inserted verbatim and never re-parsed as actions.
	tpl := template.Must(template.New("index").Parse(indexHTMLRaw))
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]string{"Morphdom": morphdomJS}); err != nil {
		panic("execute index template: " + err.Error())
	}
	indexHTML = buf.String()
}
