package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer wires up a broker, renderer, and handler for integration tests.
func newTestServer(t *testing.T) (*Handler, *Broker, func()) {
	t.Helper()
	broker := NewBroker()

	r, err := NewMarkdownRenderer()
	if err != nil {
		t.Fatalf("NewMarkdownRenderer: %v", err)
	}
	h := &Handler{
		broker:    broker,
		md:        r,
		indexHTML: renderIndex(baseStyles, r.HighlightCSS(), darkStyles),
		version:   "test",
	}
	return h, broker, func() {
		broker.Shutdown()
	}
}

// TestUpdateEndpoint verifies POST /update renders markdown and stores it as
// the latest document.
func TestUpdateEndpoint(t *testing.T) {
	h, broker, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/update", h.Update)

	body := "# Hello"
	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp updateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q", resp.Status)
	}
	if resp.Bytes != len(body) {
		t.Errorf("bytes = %d, want %d", resp.Bytes, len(body))
	}

	// The rendered document should now be available as the latest HTML.
	latest := broker.LastHTML()
	if !strings.Contains(latest, "<h1") {
		t.Errorf("LastHTML does not contain rendered heading: %s", latest)
	}
}

// TestUpdateRejectsGet verifies the /update endpoint only accepts POST.
func TestUpdateRejectsGet(t *testing.T) {
	h, _, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/update", h.Update)

	req := httptest.NewRequest(http.MethodGet, "/update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /update status = %d, want 405", rec.Code)
	}
}

// TestHealthEndpoint verifies the health response includes connection count.
func TestHealthEndpoint(t *testing.T) {
	h, _, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q", resp.Status)
	}
}

// TestStreamSendsInitialAndUpdates verifies the SSE stream sends a "connected"
// event, then the current document, then live updates.
func TestStreamSendsInitialAndUpdates(t *testing.T) {
	h, broker, cancel := newTestServer(t)
	defer cancel()

	// Seed an initial document.
	broker.Broadcast("<html>initial</html>")

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", h.Stream)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Connect and read the first two events (connected + initial update).
	// We use a short-lived reader and a deadline.
	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}

	// Read enough bytes to capture the connected + initial update events.
	// The two events may arrive in separate reads, so loop until we have both.
	buf := make([]byte, 4096)
	var out string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out += string(buf[:n])
		}
		if strings.Contains(out, "event: connected") && strings.Contains(out, "event: update") {
			break
		}
		if err != nil {
			break
		}
	}
	if !strings.Contains(out, "event: connected") {
		t.Errorf("missing connected event:\n%s", out)
	}
	if !strings.Contains(out, "event: update") {
		t.Errorf("missing initial update event:\n%s", out)
	}
	if !strings.Contains(out, "initial") {
		t.Errorf("missing initial document content:\n%s", out)
	}
}

// TestStreamLiveUpdate verifies a broadcast after a client connects is
// delivered to that client.
func TestStreamLiveUpdate(t *testing.T) {
	h, broker, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", h.Stream)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()

	// Wait for the broker to register the client before broadcasting.
	// Polling ClientCount is deterministic; a fixed sleep would be flaky on
	// slow CI runners.
	deadline := time.Now().Add(2 * time.Second)
	for broker.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if broker.ClientCount() == 0 {
		t.Fatal("client was not registered before broadcast")
	}

	// Broadcast a new document.
	broker.Broadcast("<html>live-update</html>")

	// Read in a goroutine; the SSE stream stays open so a plain Read blocks.
	// We collect output until we see the live update or time out.
	outCh := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 8192)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				if strings.Contains(out.String(), "live-update") {
					outCh <- out.String()
					return
				}
			}
			if err != nil {
				outCh <- out.String()
				return
			}
		}
	}()

	select {
	case out := <-outCh:
		if !strings.Contains(out, "live-update") {
			t.Fatalf("did not receive live update. output:\n%s", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for live update")
	}
}

// TestIndexServesUI verifies GET / serves the assembled HTML page with
// morphdom and all stylesheets inlined. The index page must contain the
// base markdown styles, the chroma syntax-highlighting CSS, and the dark
// theme overrides, because the frontend only diffs the #preview node from
// SSE updates and never applies the <style> block of the rendered document.
// Without these styles here, syntax highlighting and typography would be
// unstyled in the browser.
func TestIndexServesUI(t *testing.T) {
	h, _, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.indexHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "EventSource") {
		t.Error("index missing EventSource client code")
	}
	if !strings.Contains(body, "morphdom") {
		t.Error("index missing inlined morphdom")
	}
	if strings.Contains(body, "{{MORPHDOM}}") {
		t.Error("morphdom placeholder was not replaced")
	}

	// The base markdown styles must be present so typography/layout works
	// from the first load, before any SSE update arrives.
	if !strings.Contains(body, ".markdown-body") {
		t.Error("index missing base markdown styles (.markdown-body)")
	}
	// The chroma syntax-highlighting classes must be present so code blocks
	// are colored. WithClasses(true) emits rules like ".chroma .k { ... }".
	if !strings.Contains(body, ".chroma") {
		t.Error("index missing chroma syntax-highlighting CSS (.chroma)")
	}
	if !strings.Contains(body, ".chroma .k") {
		t.Error("index missing chroma keyword token rule (.chroma .k)")
	}
	// The dark theme overrides must be present so dark mode works.
	if !strings.Contains(body, `html[data-theme="dark"]`) {
		t.Error("index missing dark theme overrides")
	}
	// No template placeholders should survive execution.
	for _, ph := range []string{"{{ .BaseStyles }}", "{{ .HighlightCSS }}", "{{ .DarkStyles }}", "{{ .Morphdom }}"} {
		if strings.Contains(body, ph) {
			t.Errorf("index still contains placeholder %q", ph)
		}
	}
}

// TestIndexNotFound verifies non-root paths are not served as the index.
func TestIndexNotFound(t *testing.T) {
	h, _, cancel := newTestServer(t)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.indexHandler)

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestVersionEndpoint verifies the version metadata is returned.
func TestVersionEndpoint(t *testing.T) {
	h, _, cancel := newTestServer(t)
	defer cancel()
	h.version = "1.2.3"
	h.commit = "abc"
	h.buildTime = "2026-01-01"

	mux := http.NewServeMux()
	mux.HandleFunc("/version", h.Version)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp versionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("version = %q", resp.Version)
	}
}
