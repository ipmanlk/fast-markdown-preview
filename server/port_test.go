package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestPortFileWriteRead verifies the port file round-trips correctly.
func TestPortFileWriteRead(t *testing.T) {
	dir := t.TempDir()
	pf := newPortFile(dir)
	if err := pf.write(54321, 999, "1.0.0"); err != nil {
		t.Fatalf("write: %v", err)
	}

	port, pid, version, err := readPortFile(pf.path)
	if err != nil {
		t.Fatalf("readPortFile: %v", err)
	}
	if port != 54321 {
		t.Errorf("port = %d, want 54321", port)
	}
	if pid != 999 {
		t.Errorf("pid = %d, want 999", pid)
	}
	if version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", version)
	}
}

// TestPortFileRemove verifies the file is removed.
func TestPortFileRemove(t *testing.T) {
	dir := t.TempDir()
	pf := newPortFile(dir)
	_ = pf.write(1, 1, "x")
	pf.remove()
	if _, err := os.Stat(pf.path); !os.IsNotExist(err) {
		t.Fatalf("port file still exists after remove: %v", err)
	}
}

// TestReadPortFileMissing verifies a missing port file returns an error.
func TestReadPortFileMissing(t *testing.T) {
	_, _, _, err := readPortFile(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestReadPortFileMalformed verifies a malformed file returns an error.
func TestReadPortFileMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "port.txt")
	_ = os.WriteFile(path, []byte("garbage no equals here\n"), 0o644)
	_, _, _, err := readPortFile(path)
	if err == nil {
		t.Fatal("expected error for malformed file")
	}
}

// TestNegotiatePortAuto verifies port 0 returns a real listener on a free port.
func TestNegotiatePortAuto(t *testing.T) {
	l, port, err := negotiatePort(0)
	if err != nil {
		t.Fatalf("negotiatePort: %v", err)
	}
	defer l.Close()
	if port == 0 {
		t.Fatal("port = 0, want a real port")
	}
	if l.Addr().String() == "" {
		t.Fatal("listener has no address")
	}
}

// TestNegotiatePortSpecific verifies a requested port is honored.
func TestNegotiatePortSpecific(t *testing.T) {
	// First grab a free port to request explicitly.
	l, port, err := negotiatePort(0)
	if err != nil {
		t.Fatalf("negotiatePort(0): %v", err)
	}
	l.Close()

	// Now request that same port. There's a small race window but it's
	// almost always free immediately after close.
	l2, gotPort, err := negotiatePort(port)
	if err != nil {
		t.Fatalf("negotiatePort(%d): %v", port, err)
	}
	defer l2.Close()
	if gotPort != port {
		t.Errorf("gotPort = %d, want %d", gotPort, port)
	}
}

// TestDefaultDataDir verifies the data dir is non-empty and absolute.
func TestDefaultDataDir(t *testing.T) {
	d := defaultDataDir()
	if d == "" {
		t.Fatal("defaultDataDir returned empty string")
	}
	if !filepath.IsAbs(d) {
		t.Errorf("defaultDataDir = %q, want absolute", d)
	}
}

// TestDefaultDataDirMatchesPlugin ensures the Go server writes the port file
// to the same directory the Python plugin reads from on every platform. A
// mismatch (e.g. PascalCase vs kebab-case on macOS) silently breaks
// multi-window server discovery.
func TestDefaultDataDirMatchesPlugin(t *testing.T) {
	// Expected final path component per platform, matching the plugin's
	// _data_dir() in fast_markdown_preview.py.
	var wantComponent string
	switch runtime.GOOS {
	case "windows":
		wantComponent = "FastMarkdownPreview"
	case "darwin":
		wantComponent = "fast-markdown-preview"
	default: // linux, freebsd, etc.
		wantComponent = "fast-markdown-preview"
	}
	d := defaultDataDir()
	if filepath.Base(d) != wantComponent {
		t.Errorf("defaultDataDir base = %q, want %q (must match plugin _data_dir)",
			filepath.Base(d), wantComponent)
	}
}
