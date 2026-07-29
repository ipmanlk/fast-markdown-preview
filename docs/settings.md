# Settings

Settings live in `FastMarkdownPreview.sublime-settings`. Open them via
**Preferences > Package Settings > Fast Markdown Preview > Settings**, or run
**FMP: Settings** from the Command Palette.

```jsonc
{
    // Port for the preview server. 0 = auto-select a free port.
    "server_port": 0,

    // Milliseconds to wait after the last keystroke before sending markdown.
    // Lower = more responsive, higher = less CPU. 50 is a good default.
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

## Setting reference

### `server_port`

The port the preview server listens on. Defaults to `0`, which lets the
operating system pick a free port. Set this to a fixed port only if you need
to (for example, to open the preview URL manually in a specific browser).

### `debounce_ms`

How long to wait after the last keystroke before sending the markdown to the
server. The default of 50 ms feels instant while avoiding unnecessary renders
while you are still typing.

### `auto_open_browser`

When `true` (the default), the browser opens automatically the first time you
toggle the preview on. Set to `false` if you prefer to open the URL yourself.

### `server_binary_path`

Path to a custom server binary. If set, the plugin uses this binary instead of
downloading one. Useful for development or if you build the server yourself.
Leave empty for the default auto-download behavior.

### `theme`

Controls the preview theme. `"auto"` follows your operating system's
light/dark setting. Set to `"light"` or `"dark"` to force one.

### `idle_timeout_minutes`

The server shuts itself down after this many minutes with no connected
browsers and no updates. This prevents orphaned processes if you forget to
close the preview or if Sublime Text crashes. The default is 5 minutes. Set
to `0` to disable (not recommended, since it can leave processes running).

### `follow_active_view`

When `true` (the default), switching to another Markdown tab updates the
preview automatically. Set to `false` if you want the preview to stay fixed on
one file.

### `github_repo`

The GitHub repository (in `owner/name` format) to download the server binary
from. Change this only if you are running a fork.
