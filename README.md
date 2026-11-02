# servef

Browse any file in a local directory: rendered Markdown, image previews,
formatted JSON/YAML/HTML/CSS/JavaScript, and highlighted source for everything
else.

## Features

- Rendered Markdown, image previews, and highlighted, read-only source
- Format JSON, YAML, HTML, CSS, and JavaScript on demand; files stay unchanged
- Serve several directories; switch repositories and worktrees
- Live updates: the tree, open document, Source Control, and Search follow changes on disk
- VS Code-style Explorer, Search, Source Control, borderless editor tabs, and command palette
- A Welcome tab that lists your projects and shows what you can do
- Review uncommitted work, commits, a range of commits, a branch with or without its uncommitted work, or a branch stack layer by layer in one stacked diff tab
- Go to definition and find references through installed language servers
- Walkthroughs: agents publish guided tours of files and diffs with narration inline in the code, with `servef walkthrough publish` and no server needed ([agent skill](skills/servef-walkthrough/SKILL.md): `npx skills add flexdinesh/servef --skill servef-walkthrough`)
- 28 themes, including Omarchy's built-ins; Mermaid diagrams; line links
- Desktop app (macOS, Linux; preview): `servef .` opens projects in their own windows, with notifications for branch switches, commits, settled edits, and walkthroughs; it also opens servers started in a terminal and pasted `http://localhost` links, folders from Finder or Linux file managers, and can start at login

## Install

### Install script

macOS and Linux, no sudo; installs `servef` in `~/.local/bin` (or `~/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh
```

The desktop app and the CLI:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop.sh | sh
```

Remove either with `scripts/uninstall.sh` the same way.

### Homebrew

```sh
brew install flexdinesh/tap/servef
```

### Go

Requires Go 1.26 or later.

Latest stable release:

```sh
go install github.com/flexdinesh/servef@latest
```

Specific stable version:

```sh
go install github.com/flexdinesh/servef@v0.1.3
```

Latest `main` with embedded frontend assets:

```sh
go install github.com/flexdinesh/servef@edge
```

These installs include the embedded frontend; Node.js and pnpm are not required.
Other branches and commits do not include it and fail to build with
`go install`.

### Desktop app

Optional: macOS and Linux packages on
[GitHub Releases](https://github.com/flexdinesh/servef/releases). See
[desktop](site/src/pages/docs/desktop.mdx) for install and Gatekeeper steps.

## Usage

Open the current directory:

```sh
servef .
```

With the desktop app installed, `servef [path]` opens it there and exits;
otherwise (or with `--host`/`--port`) it serves in a browser. Over SSH,
`servef remote [path]` (and `servef [path]`) prints a `servef://ssh/…` link
for the app on your computer ([remote hosts](site/src/pages/docs/remote.mdx)).
`servef browser [path]` always serves in a browser and takes the same flags.
To open a directory named `browser`, run `servef ./browser`.

See all options:

```sh
servef --help
```

## Documentation

Usage, desktop app, browsing, code navigation, and walkthrough guides live in
[`site/src/pages/docs`](https://github.com/flexdinesh/servef/tree/main/site/src/pages/docs).
Contributors: see [development](docs/development.md) and
[architecture](docs/architecture.md).
