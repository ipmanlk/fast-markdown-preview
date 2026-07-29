# Troubleshooting

## The browser does not open

1. Check the Sublime Text console (`View > Show Console`) for errors.
2. Make sure you have a default browser set in your operating system.
3. Try running **FMP: Reopen Preview** to restart the server and open a fresh
   tab.

## "FMP: server is not running"

This means the server failed to start or crashed. Check the console for
details. Common causes:

- The server binary could not be downloaded (no internet connection on first
  use). Connect to the internet and try again, or build the binary manually
  (see [Development](development.md)).
- A firewall is blocking localhost connections. Allow connections to
  `127.0.0.1`.

## The preview is not updating

1. Check the status dot in the bottom-right of the browser. If it is yellow
   or red, the connection dropped. Run **FMP: Reopen Preview**.
2. Make sure you are editing a Markdown file (the toggle only works on
   `.md` files).
3. If `follow_active_view` is on, make sure the file you are editing is the
   active tab.

## "SyntaxError" about type annotations in the console

This means Sublime Text is using its legacy Python 3.3 host instead of the
modern 3.8 host. Make sure the `.python-version` file is present in the
package directory with the content `3.8`. This file is included in the repo,
but it can be missing if you installed an old version or copied files
manually.

## The server binary download failed

The plugin downloads the binary from GitHub Releases on first use. If this
fails:

1. Check your internet connection.
2. Check that you can reach `github.com` and `api.github.com`.
3. If you are behind a proxy, make sure it is configured for Sublime Text.
4. As a fallback, you can build the binary yourself and set
   `server_binary_path` in the settings to point at it. See
   [Development](development.md) for build instructions.

## Sublime Text feels slow

- Increase `debounce_ms` (default 50). Higher values mean fewer renders.
- Make sure `idle_timeout_minutes` is not `0` (the server should shut down
  when idle).

## Leftover server processes

The server is designed to clean up after itself:

- It shuts down after the idle timeout (5 minutes by default) with no
  connected browsers.
- It shuts down if no browser ever connects within 2 minutes of starting.
- Sublime Text kills it on exit.

If you still find orphaned `fast-md-preview` processes, you can kill them
manually. The process name is `fast-md-preview` (or `fast-md-preview.exe` on
Windows).
