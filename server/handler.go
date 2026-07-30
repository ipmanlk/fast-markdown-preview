package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// maxBodyBytes caps the size of a /update payload. Markdown files are rarely
// this large; the limit protects the server from unbounded memory use.
const maxBodyBytes = 10 << 20 // 10 MiB

// Handler holds the shared dependencies wired into every HTTP route.
type Handler struct {
	broker    *Broker
	md        *MarkdownRenderer
	indexHTML string // fully-assembled browser UI, inlined once at startup
	version   string
	commit    string
	buildTime string
}

// updateResponse is the JSON body returned by POST /update.
type updateResponse struct {
	Status    string `json:"status"`
	Bytes     int    `json:"bytes"`
	Timestamp int64  `json:"timestamp"`
}

// healthResponse is returned by GET /health.
type healthResponse struct {
	Status            string `json:"status"`
	ActiveConnections int    `json:"active_connections"`
	UptimeSeconds     int64  `json:"uptime_seconds"`
}

// versionResponse is returned by GET /version.
type versionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// startTime is captured at process start for uptime reporting.
var startTime = time.Now()

// Update handles POST /update. It reads the raw markdown body, renders it to a
// full HTML document, and broadcasts the result to all connected browsers.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// LimitReader caps the read at maxBodyBytes. If the request body is
	// larger, we reject it with 413 so the caller knows the payload was
	// truncated rather than silently rendering a partial document.
	limited := io.LimitReader(r.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body) > maxBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	html, err := h.md.Render(body)
	if err != nil {
		slog.Warn("render failed", "err", err, "bytes", len(body))
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	h.broker.Broadcast(html)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updateResponse{
		Status:    "ok",
		Bytes:     len(body),
		Timestamp: time.Now().Unix(),
	})
}

// Stream handles GET /stream, the SSE endpoint. It registers a client channel
// with the broker, sends the current document immediately, then fans out
// subsequent broadcasts until the client disconnects or the server shuts down.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

	// Buffered channel absorbs short bursts without blocking the broker.
	ch := make(chan string, 8)
	h.broker.Register(ch)
	defer func() {
		h.broker.Unregister(ch)
	}()

	// Send a "connected" event so the browser can flip its status indicator
	// immediately, then push the current document so the page is never blank.
	writeSSE(w, flusher, "connected", `{"status":"ok"}`)
	if latest := h.broker.LastHTML(); latest != "" {
		writeSSE(w, flusher, "update", latest)
	}

	for {
		select {
		case <-r.Context().Done():
			// Client closed the tab or navigated away.
			return
		case html, ok := <-ch:
			if !ok {
				// Channel closed by the broker during shutdown.
				return
			}
			writeSSE(w, flusher, "update", html)
		}
	}
}

// writeSSE writes a single named SSE event and flushes it. The payload is split
// across multiple "data:" lines so embedded newlines survive the wire format.
func writeSSE(w http.ResponseWriter, f http.Flusher, event, payload string) {
	// "event:" line, then one or more "data:" lines, then a blank line.
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, splitSSELines(payload))
	f.Flush()
}

// Health handles GET /health. The ST plugin uses this for liveness checks and
// startup coordination.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:            "ok",
		ActiveConnections: h.broker.ClientCount(),
		UptimeSeconds:     int64(time.Since(startTime).Seconds()),
	})
}

// Version handles GET /version. Values are injected via ldflags at build time.
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(versionResponse{
		Version:   h.version,
		Commit:    h.commit,
		BuildTime: h.buildTime,
	})
}

// indexHandler serves the embedded browser UI. A query param ?theme=light|dark
// is read by the page itself; the server does not need to interpret it.
func (h *Handler) indexHandler(w http.ResponseWriter, r *http.Request) {
	// Guard against the default mux matching arbitrary paths to "/". Only serve
	// the index for the root path, everything else is a 404.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, h.indexHTML)
}
