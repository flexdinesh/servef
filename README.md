# servef

Browse any file in a local directory: rendered Markdown, image previews,
formatted JSON/YAML/HTML/CSS/JavaScript, and highlighted source for everything
else.

## Features

- Rendered Markdown, image previews, and highlighted, read-only source
- Format JSON, YAML, HTML, CSS, and JavaScript on demand; files stay unchanged
- Serve several directories; switch repositories and worktrees
- VS Code-style Explorer, Search, Source Control, tabs, and command palette
- Diff uncommitted work, commits, a whole branch, or a range of commits
- Go to definition and find references through installed language servers
- Themes, Mermaid diagrams, and line links

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

Latest `main` that passed CI:

```sh
go install github.com/flexdinesh/servef@edge
```

These installs include the embedded frontend; Node.js and pnpm are not required.
Other branches and commits do not include it and fail to build with
`go install`.

## Usage

Serve the current directory:

```sh
servef .
```

See all options:

```sh
servef --help
```

## Documentation

Usage, browsing, and code navigation guides live in
[`site/src/pages`](https://github.com/flexdinesh/servef/tree/main/site/src/pages).
Contributors: see [development](docs/development.md) and
[architecture](docs/architecture.md).
