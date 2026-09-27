# servef

Serve and browse Markdown files from a local directory.

## Features

- Render Markdown content and local images
- Switch each open document between preview and read-only source
- Navigate files in a compact, resizable tree
- Keep visited documents in an editor-style tab bar
- Show document details and supported process CPU/RAM in a status line
- Switch between system, light, dark, and editor palettes
- Render Mermaid diagrams with Excalidraw

## Install

### Homebrew

```sh
brew install flexdinesh/tap/servef
```

### Go

Requires Go 1.24 or later.

Latest stable release:

```sh
go install github.com/flexdinesh/servef@latest
```

Specific stable version:

```sh
go install github.com/flexdinesh/servef@v0.1.3
```

All Go installs include the embedded frontend; Node.js and pnpm are not required.

## Usage

Serve the current directory:

```sh
servef .
```

Or serve another directory:

```sh
servef path/to/markdown
```

Without `--port`, servef uses the first free port from `7971` through `7980`.
Use `--host` to bind a specific IP address or `--port` to select an exact port.
If the range is full, servef can stop all verified instances after confirmation.

See all options:

```sh
servef --help
```
