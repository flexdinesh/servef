<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/mark-dark.svg">
  <img src="docs/brand/mark-light.svg" alt="Source Zero logo" width="64" height="64">
</picture>

# Source Zero

Source Zero (`srcz`) browses local files: rendered Markdown, image previews, formatted
JSON/YAML/HTML/CSS/JavaScript, highlighted source, Source Control diffs, code
navigation, walkthroughs, and live updates. Desktop app, browser, or remote.

## Install

macOS and Linux, no sudo, no runtime needed. Installs to `~/.local/bin` (or `~/bin`).

```sh
# CLI and desktop app (Linux needs WebKitGTK 4.1)
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh
# CLI only
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh -s -- --cli-only
# uninstall everything (keeps user data)
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/uninstall.sh | sh
```

Run a script again to update. Latest `main` builds: see [development](docs/development.md#install-from-main).

## Use

```sh
srcz .            # open in the desktop app (falls back to the browser)
srcz browser .    # always serve in the browser
srcz . --remote   # serve to another machine; run the printed `srcz connect ...` there
srcz --help
```

## Docs

- [CLI](docs/cli.md)
- [User guides](apps/site/src/pages/docs)
- [Development](docs/development.md), [architecture](docs/architecture.md), [release](docs/release.md)
- Agent skill: `npx skills add flexdinesh/srcz --skill srcz-walkthrough`
