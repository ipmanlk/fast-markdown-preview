# Development

This guide is for people working on the plugin itself. If you just want to
use it, see the [README](../README.md) and [Installation](installation.md).

## Repository layout

```
fast-markdown-preview/
  fast_markdown_preview.py            # The Sublime Text 4 plugin
  FastMarkdownPreview.sublime-settings
  FastMarkdownPreview.sublime-commands
  Main.sublime-menu
  Default (Linux|Windows|OSX).sublime-keymap
  package.json                        # Package Control metadata
  dependencies.json                   # Empty: no Python deps
  .no-sublime-package                 # Keeps the package unpacked on disk

  server/                             # The Go preview server
    main.go           # Entrypoint, flags, signal handling, watchdogs
    broker.go         # SSE broker (channel per client, slow-client drop)
    handler.go        # HTTP handlers (/update, /stream, /health, /version, /)
    markdown.go       # goldmark setup and Render()
    port.go           # Port negotiation and port file
    styles.go         # Inlined CSS (light + dark themes)
    embed.go          # Embeds frontend/index.html and morphdom into the binary
    frontend/
      index.html      # The browser UI (single self-contained file)
      morphdom.min.js # morphdom v2.7.4, vendored
    go.mod / go.sum

  tests/
    conftest.py         # Stubs out sublime/sublime_plugin so tests can import the plugin
    test_plugin.py      # Unit tests for the plugin's pure logic
    test_integration.py # End-to-end tests that build the Go server and exercise the real HTTP API

  .goreleaser.yml      # Cross-compiles the server for 6 platform combos
  .github/workflows/
    build.yml           # CI: build, vet, test on every PR and push to main
    release.yml         # CD: on a vX.Y.Z tag, build and publish a GitHub Release
  Makefile
```

## The two components

The project has two independent pieces that talk over localhost HTTP:

1. **The Go server** (`server/`). A standalone binary that listens on
   `127.0.0.1`, parses Markdown with goldmark, and streams rendered HTML to
   browsers over SSE. It knows nothing about Sublime Text.

2. **The Sublime plugin** (`fast_markdown_preview.py`). It listens for edits
   and view switches, manages the server process, posts Markdown to it, and
   opens the browser.

You can develop and test each side on its own.

## Prerequisites

- **Go 1.22 or newer** to build the server.
- **Python 3.8 or newer** for the plugin. Sublime Text ships its own Python,
  so you only need a system Python for running the tests.
- **pytest** for the Python tests: `pip install pytest`.
- **GoReleaser** (optional) only if you want to test a full release build
  locally. Install it from https://goreleaser.com/install/.

## Building and running the server

```bash
make build     # builds server/bin/fast-md-preview
make run       # builds, then runs on a free port with idle timeout off
```

While it runs you can poke at it directly:

```bash
# The port is printed on the first line of stdout, e.g. PORT=34567
curl http://127.0.0.1:34567/health
curl http://127.0.0.1:34567/version

# Send some Markdown
curl -X POST http://127.0.0.1:34567/update \
     -H "Content-Type: text/plain" \
     --data-binary '# Hello

This is **bold**.'

# Open http://127.0.0.1:34567/ in a browser to see the preview.
# The SSE stream lives at /stream.
```

Server flags:

| Flag | Default | Purpose |
|---|---|---|
| `--port` | `0` | Port to listen on. `0` lets the OS pick a free one. |
| `--idle-timeout` | `5m` | Shut down after this long with no SSE clients and no updates. `0` disables. |
| `--connection-timeout` | `2m` | Shut down if no browser connects within this time of startup. `0` disables. Catches orphaned servers when the browser never opens. |
| `--data-dir` | platform default | Where to write the port file. |
| `--verbose` | off | Enable debug logging. |

## HTTP API

| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Serves the embedded browser UI. |
| GET | `/stream` | SSE stream. Sends a `connected` event, then the current document, then `update` events as Markdown changes. |
| POST | `/update` | Accepts a raw Markdown body (text/plain, max 10 MB), renders it, and broadcasts to all connected browsers. |
| GET | `/health` | Liveness probe. Returns JSON with status, connection count, and uptime. |
| GET | `/version` | Build version, commit, and time (injected via ldflags). |

## Running the tests

```bash
make test          # Go tests + Python tests
make test-go       # Go tests only (run with -race in CI)
make test-python   # Python tests only (unit + integration)
```

The Go tests cover the broker (broadcast, slow-client drop, shutdown), the
Markdown renderer (every extension, SSE line splitting, CSS sanity), the HTTP
handlers (update, stream live updates, health, version, index), and port
negotiation.

The Python tests come in two flavors:

- **Unit tests** (`tests/test_plugin.py`) stub out the `sublime` and
  `sublime_plugin` modules (see `tests/conftest.py`) so the plugin's pure
  logic can be tested without Sublime. They cover platform detection, archive
  naming, port-file parsing, checksum verification, debounce, adopted-server
  detection, and view detection.
- **Integration tests** (`tests/test_integration.py`) build the real Go
  server, start it on a free port, and exercise the full pipeline: health
  check, posting Markdown through the plugin's HTTP client, and reading the
  SSE stream to confirm the rendered HTML arrives.

## Local install for development

```bash
make install    # builds the server and copies the plugin into your ST Packages dir
```

This copies the plugin files and the freshly built server binary into
`Packages/FastMarkdownPreview/server/`. The plugin checks this directory
first before attempting a download, so your local build is used automatically.
Restart Sublime Text (or use `fmp_reopen`) after running this.

The `.python-version` file (containing `3.8`) is included so that ST4 uses
its modern Python 3.8 host when loading the plugin.

## Code conventions

- Go: run `make fmt` and `make lint` before committing. Standard gofmt
  formatting, idiomatic Go.
- Python: standard library only. No external dependencies. Keep it compatible
  with Sublime Text's embedded Python (3.8+).
- Comments: plain English. No fancy unicode, no decorative separators.

## Critical invariants

Do not break these. Each is enforced by a test where noted.

- **Data directory paths must match across the Go server and the Python
  plugin on every platform.** `defaultDataDir()` in `server/port.go` must
  return the same directory as `_data_dir()` in `fast_markdown_preview.py`
  (kebab-case on Linux/macOS, PascalCase `FastMarkdownPreview` on Windows).
  Enforced by the Go test `TestDefaultDataDirMatchesPlugin`.
- **Release archive names must match.** The `name_template` in
  `.goreleaser.yml` must produce names that `_archive_name()` in
  `fast_markdown_preview.py` expects:
  `fast-md-preview_<version>_<os>_<arch>.tar.gz` (or `.zip` on Windows). If
  you change one, change the other and update the Python tests.
- **`.python-version` must contain `3.8`.** It forces Sublime Text 4 to load
  the plugin on its Python 3.8 host so the type annotations parse.
- **The server binds to `127.0.0.1` only.** It must never be reachable from
  the network. `negotiatePort` in `server/port.go` hard-codes this.
- **The browser UI must stay self-contained.** Inline all CSS and JS. No CDN
  requests, no external resources. This is what lets the preview work offline
  and keeps morphdom diffing stable.

## How the server gets onto a user's machine

The plugin does not ship the Go binary in the repo. Instead:

1. On first preview, the plugin asks the GitHub API for the latest release tag.
2. It downloads the platform-specific archive, for example
   `fast-md-preview_1.0.0_linux_amd64.tar.gz`. Archive names are built by
   `_archive_name()` in the plugin and must match what GoReleaser produces
   (see `.goreleaser.yml`). If you change one, change the other.
3. It downloads `checksums.txt`, looks up the hash for that archive, and
   verifies the SHA256.
4. It extracts the binary into the cache directory (see below), makes it
   executable on Unix, and writes the version to `version.txt`.
5. Subsequent loads skip the download if the cached version matches the
   latest tag. If GitHub is unreachable, it falls back to the cached binary.

### Where things live at runtime

| What | Linux | macOS | Windows |
|---|---|---|---|
| Server binary (cache) | `~/.cache/sublime-text/Packages/FastMarkdownPreview/server/` | `~/Library/Caches/Sublime Text/Packages/FastMarkdownPreview/server/` | `%LOCALAPPDATA%\Sublime Text\Cache\Packages\FastMarkdownPreview\server\` |
| Port file + locks | `~/.local/share/fast-markdown-preview/` | `~/Library/Application Support/fast-markdown-preview/` | `%APPDATA%\FastMarkdownPreview\` |

The binary cache uses `sublime.cache_path()` so it survives package updates.
The port file lives outside the package dir for the same reason and so
multiple Sublime windows can find the shared server.
