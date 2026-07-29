# Fast Markdown Preview

A live-preview plugin for Sublime Text 4. It renders your Markdown to styled
HTML in a browser window that updates as you type.

You type in Sublime, the browser preview refreshes within ~50 ms. Switch to
another Markdown tab and the preview follows it.

It works by running a small Go server on localhost that parses Markdown with
[goldmark](https://github.com/yuin/goldmark) and streams the rendered HTML to
the browser over Server-Sent Events. The browser patches only the changed DOM
nodes with [morphdom](https://github.com/patrick-steele-idem/morphdom), so
updates are fast and your scroll position is preserved.

## Features

- **Instant updates.** The preview refreshes within ~50 ms of your last
  keystroke.
- **Follows the active view.** Switch tabs or open another `.md` file and the
  preview tracks it automatically.
- **Full Markdown support.** GFM tables, strikethrough, task lists, definition
  lists, footnotes, typographer (smart quotes and dashes), autolinks, emoji
  shortcodes, auto heading IDs, and syntax-highlighted code blocks.
- **Light and dark themes.** Follows your OS preference, or force one with
  `?theme=light` / `?theme=dark`.
- **Zero config.** The server binary is auto-downloaded from GitHub Releases
  on first use, and the port is auto-selected.
- **Low resource usage.** A single ~13 MB static Go binary. SSE uses no
  bandwidth when idle, and the server auto-shuts down after inactivity.
- **Cross-platform.** Linux, macOS, and Windows, both amd64 and arm64.
- **No dependencies.** The plugin uses only the Python standard library, and
  the browser UI is a single self-contained HTML file.

## Installation

Clone this repository into your Sublime Text `Packages/` directory:

```bash
# macOS
cd ~/Library/Application\ Support/Sublime\ Text/Packages/

# Linux
cd ~/.config/sublime-text/Packages/

# Windows (in PowerShell)
cd $env:APPDATA\Sublime Text\Packages\

git clone https://github.com/ipmanlk/fast-markdown-preview.git FastMarkdownPreview
```

Restart Sublime Text. The server binary is downloaded automatically on first
preview.

If you see a `SyntaxError` about type annotations in the ST console, make sure
`.python-version` is present in the package directory. It tells ST4 to use its
Python 3.8 host instead of the legacy 3.3 host.

See [docs/installation.md](docs/installation.md) for more detail.

## Usage

1. Open a Markdown (`.md`) file.
2. Run **Fast Markdown Preview: Toggle Preview** from the Command Palette, or
   press `Ctrl+Alt+M` (Windows/Linux) / `Cmd+Alt+M` (macOS).
3. Your default browser opens with the rendered preview.
4. Type. The preview updates as you go.
5. Switch to another Markdown tab. The preview follows it.

### Commands

| Command | Palette entry | Default key |
|---|---|---|
| Toggle preview | FMP: Toggle Preview | `Ctrl+Alt+M` (Win/Linux), `Cmd+Alt+M` (macOS) |
| Reopen preview | FMP: Reopen Preview | |
| Settings | FMP: Settings | |

## Configuration

Settings live in `FastMarkdownPreview.sublime-settings`. Open them via
**Preferences > Package Settings > Fast Markdown Preview > Settings**.

```jsonc
{
    // Port for the preview server. 0 = auto-select a free port.
    "server_port": 0,

    // Milliseconds to wait after the last keystroke before sending markdown.
    "debounce_ms": 50,

    // Automatically open the browser when the preview is toggled on.
    "auto_open_browser": true,

    // Override path to a custom server binary. If empty, auto-download.
    "server_binary_path": "",

    // Preview theme: "auto" (follow OS), "light", or "dark".
    "theme": "auto",

    // Idle timeout in minutes. Server auto-shuts down after this long with no
    // connected browsers and no updates. 0 = never (not recommended).
    "idle_timeout_minutes": 5,

    // Follow the active Markdown view: switching tabs updates the preview.
    "follow_active_view": true,

    // GitHub repo for binary downloads (owner/name).
    "github_repo": "ipmanlk/fast-markdown-preview"
}
```

## How it works

```
Sublime Text 4 (Python plugin)
  |  on_modified_async (debounced ~50ms) + on_activated_async (follow view)
  |  HTTP POST /update  (localhost)
  v
Go server (single static binary)
  |  goldmark parse -> HTML
  |  SSE broadcast to all connected browsers
  v
Browser UI (single HTML page, embedded in the binary)
  |  EventSource -> morphdom DOM diff (only changed nodes are repainted)
```

- The plugin spawns the Go server as a subprocess on first preview. The server
  binds to `127.0.0.1` only, never to the network, and picks a free port.
- The port is written to a well-known file so multiple Sublime windows share
  one server.
- The server auto-shuts down after 5 minutes (configurable) with no connected
  browsers and no updates, so no orphaned processes are left behind.
- If Sublime crashes, the idle timeout cleans up the server.

For the details behind these decisions, see
[docs/development.md](docs/development.md).

## Development

Prerequisites: Go 1.22+ and Python 3.8+ (with pytest for the tests).

```bash
make build      # build the server into server/bin/
make test       # run Go + Python tests
make run        # build and run the server on a free port
make fmt        # format Go code
make lint       # vet Go code
make install    # copy the plugin into your ST Packages dir
```

For the full development guide, see [docs/development.md](docs/development.md).
Other docs:

- [Installation](docs/installation.md)
- [Usage](docs/usage.md)
- [Settings](docs/settings.md)
- [Troubleshooting](docs/troubleshooting.md)

## License

[MIT](LICENSE)
