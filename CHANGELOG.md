# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-07-29

### Added
- Initial release.
- Live Markdown preview in your browser that updates as you type, refreshing
  within ~50 ms of your last keystroke.
- Full GFM Markdown support: tables, strikethrough, task lists, definition
  lists, footnotes, smart quotes and dashes, autolinks, emoji shortcodes, auto
  heading IDs, and syntax-highlighted code blocks.
- The preview follows the active Markdown tab: switching files or opening
  another `.md` file updates the preview automatically.
- Light and dark themes that follow your OS preference, or force one with
  `?theme=light` / `?theme=dark`.
- The preview server binary is auto-downloaded from GitHub Releases on first
  use and verified against a published SHA256 checksum. No manual setup.
- One server instance is shared across all Sublime Text windows.
- The server auto-shuts down after a configurable idle period and cleans up
  when Sublime Text exits, so no processes are left behind.
- Cross-platform support: Linux, macOS, and Windows, on both amd64 and arm64.
- The plugin uses only the Python standard library, and the browser UI is a
  single self-contained page that works offline.
