# servef

Browse local files: rendered Markdown, image previews, formatted
JSON/YAML/HTML/CSS/JavaScript, highlighted source, Source Control diffs, code
navigation, walkthroughs, and live updates. Desktop app, browser, or remote.

## Install

macOS and Linux, no sudo, no runtime needed. Installs to `~/.local/bin` (or `~/bin`).

```sh
# CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh
# desktop app and CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-electron.sh | sh
# native app (experimental)
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-native.sh | sh
# uninstall everything (keeps user data)
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/uninstall.sh | sh
```

Run a script again to update. Latest `main` builds: see [development](docs/development.md#install-from-main).

## Use

```sh
servef .            # open in the desktop app (falls back to the browser)
servef browser .    # always serve in the browser
servef --help
```

## Docs

- [CLI](docs/cli.md)
- [User guides](apps/site/src/pages/docs)
- [Development](docs/development.md), [architecture](docs/architecture.md), [release](docs/release.md)
- Agent skill: `npx skills add flexdinesh/servef --skill servef-walkthrough`
