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
- Desktop app (macOS, Linux; preview): `servef .` opens projects in their own windows, with notifications for branch switches, commits, settled edits, and walkthroughs; it also opens servers started in a terminal and pasted `http://localhost` links, folders from Finder or Linux file managers (or dropped on a window), with recent folders, native file menus, and Reveal in File Manager, and can start at login

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

Latest `main`, rebuilt after every green push (the `edge` prerelease):

```sh
# CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-edge.sh | sh
# desktop app and CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop-edge.sh | sh
```

Remove any of them with `scripts/uninstall.sh` the same way. Run a script
again to update, or a stable one to leave edge. The scripts are the only
supported install; Homebrew and `go install` are no longer offered
(decision 0079). Installs need no Node.js, Go, or other runtime.

### Desktop app

Optional: macOS and Linux packages on
[GitHub Releases](https://github.com/flexdinesh/servef/releases). See
[desktop](apps/site/src/pages/docs/desktop.mdx) for install and Gatekeeper steps.

## Usage

Open the current directory:

```sh
servef .
```

With the desktop app installed, `servef [path]` opens it there and exits;
otherwise (or with `--host`/`--port`) it serves in a browser. On a machine
without a UI (an SSH session, or Linux without a display) it serves on that
machine's address and prints a `web:` URL and a `desktop:` `servef://` URL to
open from another computer; that server has no authentication
([remote hosts](apps/site/src/pages/docs/remote.mdx)). The desktop app can also
connect to SSH hosts itself. `servef browser [path]` always serves on
localhost and takes the same flags.
To open a directory named `browser`, run `servef ./browser`.

See all options:

```sh
servef --help
```

## Documentation

Usage, desktop app, browsing, code navigation, and walkthrough guides live in
[`apps/site/src/pages/docs`](https://github.com/flexdinesh/servef/tree/main/apps/site/src/pages/docs).
Contributors: see [development](docs/development.md) and
[architecture](docs/architecture.md).
