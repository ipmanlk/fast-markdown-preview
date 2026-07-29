# Installation

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

Restart Sublime Text. On first use, the plugin downloads the correct server
binary for your platform from GitHub Releases. This needs an internet
connection the first time. After that, the binary is cached and no network
access is required.

## The `.python-version` file

The package includes a `.python-version` file containing `3.8`. Sublime Text 4
ships two Python hosts: a legacy 3.3 host and a modern 3.8 host. Without this
file, manually installed packages default to the 3.3 host, which cannot parse
the type annotations used in this plugin. The file forces ST4 to use the 3.8
host.

If you see a `SyntaxError` about type annotations in the ST console
(`View > Show Console`), make sure this file is present in the package
directory.

## Verifying it works

1. Open a Markdown (`.md`) file.
2. Run **FMP: Toggle Preview** from the Command Palette, or press
   `Ctrl+Alt+M` (Windows/Linux) / `Cmd+Alt+M` (macOS).
3. Your default browser should open and show the rendered preview.

If nothing happens, check the Sublime Text console (`View > Show Console`)
for error messages, and see [Troubleshooting](troubleshooting.md).
