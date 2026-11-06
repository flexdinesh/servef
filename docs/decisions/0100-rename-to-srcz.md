# 0100: Rename servef to srcz (Source Zero)

- Status: Accepted
- Date: 2026-11-06
- Revises: [0095](0095-desktop-app-is-mygo-electron-removed.md) (install scripts
  no longer clean up the Electron app or `servef-mygo`),
  [0073](0073-install-scripts.md) (uninstall covers srcz only)

## Context

The product is now Source Zero, with the command `srcz`. The repository becomes
`flexdinesh/srcz`.

## Decision

- **Display name:** Source Zero in window titles, themes, the site, and user
  docs. `srcz` is the command, every identifier and path, CLI output, code
  comments, and contributor docs. The desktop launcher stays `srcz-desktop`. The
  status bar's home link says `srcz`: the full name pushes Settings off a 320 px
  screen.
- **Everything renamed, no aliases:**

  | Was | Now |
  | --- | --- |
  | `servef`, `servef-desktop` | `srcz`, `srcz-desktop` |
  | `github.com/flexdinesh/servef` | `github.com/flexdinesh/srcz` |
  | `apps/servef`, `@servef/*` | `apps/srcz`, `@srcz/*` |
  | `servef://`, `servef-app://` | `srcz://`, `srcz-app://` |
  | `window.servefDesktop` | `window.srczDesktop` |
  | `/_servef/control`, `X-Servef-*`, `servef_remote` | `/_srcz/control`, `X-Srcz-*`, `srcz_remote` |
  | `SERVEF_*` | `SRCZ_*` |
  | `io.github.flexdinesh.servef.desktop` | `io.github.flexdinesh.srcz.desktop` |
  | `servef-*` storage keys | `srcz-*` |
  | `servef-validation-engine-v0.21.10` | `srcz-validation-engine-v0.21.10` |

- **Clean break:** no migration. Config, cache, and socket directories,
  localStorage keys, and the remote identity all start fresh, so saved roots,
  connections, walkthroughs, settings, and tabs reset, and existing connection
  strings stop working.
- **No cleanup of earlier installs:** install scripts leave `servef` and
  `servef-desktop` alone, and remove no Electron or `servef-mygo` files.
  `srcz://` has no earlier handler to conflict with.
- **Records rewritten once:** this is the one exception to "supersede instead of
  rewriting history". Records 0001-0099 and their filenames use the new name,
  including for events that happened under the old one. This record alone keeps
  the old name, for search.
- **Release:** the rename starts the `0.2` series. Stable install scripts need
  `srcz_*` assets, which `v0.1.6` lacks, so `v0.2.0` is tagged right after merge.

## Consequences

- Stable installs fail between merge and the `v0.2.0` tag.
- `servef` stays installed beside `srcz` until removed by hand. `scripts/uninstall.sh`
  on `main` no longer knows it; use the script from the last commit before the
  rename, `c41fadb5`.
- Earlier releases keep their `servef_*` assets.
- The old validation engine and its volume stay until their owner removes them.
