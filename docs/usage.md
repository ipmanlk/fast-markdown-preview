# Usage

## Starting the preview

1. Open a Markdown (`.md`) file in Sublime Text.
2. Run **FMP: Toggle Preview** from the Command Palette, or press
   `Ctrl+Alt+M` (Windows/Linux) / `Cmd+Alt+M` (macOS).
3. Your default browser opens with the rendered preview.
4. Type in Sublime. The preview updates as you go.

## Following the active view

By default, the preview follows the active Markdown tab. Switch to another
`.md` file and the preview switches to it automatically. You can turn this
off with the `follow_active_view` setting (see [Settings](settings.md)).

## Stopping the preview

Run **FMP: Toggle Preview** again to turn the preview off. The server keeps
running so that toggling back on is instant, and shuts itself down after the
idle timeout (5 minutes by default) with no connected browsers.

## Reopening the preview

If the preview gets stuck or you want a fresh browser tab, run
**FMP: Reopen Preview**. This kills the server, restarts it, and opens a new
browser tab.

## Commands

| Command | Palette entry | Default key |
|---|---|---|
| Toggle preview | FMP: Toggle Preview | `Ctrl+Alt+M` / `Cmd+Alt+M` |
| Reopen preview | FMP: Reopen Preview | |
| Settings | FMP: Settings | |

All commands are also available under **Tools > FMP** in the menu bar.

## What the status dot means

The bottom-right corner of the browser preview has a small colored dot:

| Color | Meaning |
|---|---|
| Gray | Connecting |
| Green | Connected and live |
| Yellow (pulsing) | Reconnecting after a drop |
| Red | Disconnected |

Hover the dot to see the text label.

## Themes

The preview follows your OS light/dark preference by default. You can force a
theme by adding `?theme=light` or `?theme=dark` to the preview URL, or by
setting `"theme"` in the settings (see [Settings](settings.md)).
