package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// defaultDataDir returns the platform-appropriate directory for the port file
// and lock files. These live outside the ST package/cache dirs so they survive
// package updates and are shared across all ST windows.
func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "FastMarkdownPreview")
	case "darwin":
		home, _ := os.UserHomeDir()
		// Must match the plugin's _data_dir() on every platform. The plugin
		// uses the kebab-case name on Linux and macOS, and PascalCase only on
		// Windows (where it follows the APPDATA convention). A mismatch here
		// breaks multi-window port-file discovery.
		return filepath.Join(home, "Library", "Application Support", "fast-markdown-preview")
	default: // linux, freebsd, etc.
		// Respect XDG_DATA_HOME if set.
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "fast-markdown-preview")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "fast-markdown-preview")
	}
}

// negotiatePort resolves the listening port. If wanted is 0, the kernel picks a
// free port. We bind 127.0.0.1 only: the server must never be reachable from
// the network.
//
// We return the *listener* rather than just the port number so there is no gap
// between picking a port and serving on it (which would invite a race where
// another process grabs the port in between).
func negotiatePort(wanted int) (net.Listener, int, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", wanted)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, fmt.Errorf("listen on %s: %w", addr, err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	return l, port, nil
}

// portFile is the well-known file the ST plugin reads to discover the running
// server's port and PID. Format is simple key=value lines for easy parsing
// from Python without a JSON dependency.
type portFile struct {
	path string
}

func newPortFile(dataDir string) *portFile {
	return &portFile{path: filepath.Join(dataDir, "port.txt")}
}

func (p *portFile) write(port, pid int, version string) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf("port=%d\npid=%d\nversion=%s\n", port, pid, version)
	// Atomic write: temp file + rename so a concurrent reader never sees a
	// half-written file.
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

func (p *portFile) remove() {
	_ = os.Remove(p.path)
}

// readPortFile parses a port file written by the server. Returns the port and
// pid. Used by tests; the ST plugin has its own parser.
func readPortFile(path string) (port, pid int, version string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "port":
			port, err = strconv.Atoi(val)
		case "pid":
			pid, err = strconv.Atoi(val)
		case "version":
			version = val
		}
		if err != nil {
			return 0, 0, "", err
		}
	}
	if port == 0 {
		return 0, 0, "", fmt.Errorf("port not found in %s", path)
	}
	return port, pid, version, nil
}
