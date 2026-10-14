# servef

<!-- impeccable:product-schema 1 -->

## Platform

web

## Product Purpose

Browse and search any local file in a directory through a Go CLI and embedded React UI: Markdown and images preview, JSON and YAML format on demand, and other text opens as highlighted source.

## Capabilities and Constraints

- One self-contained Go binary; JavaScript tooling is build-time only.
- Runtime fonts, icons, diagrams, and other assets are bundled locally.
- Project switching: discovered git repositories and worktrees, or any folder under the served root, loaded fresh on each switch.
- File browsing, Markdown/image preview, on-demand formatting, highlighted source, document tabs, deep links, and browser history.
- `--filename` globs set CLI scope; the explorer filename filter limits visibility only. File types decide presentation, not availability.
- Separate file/path search and document-content search.
- File-tree selection follows editor conventions rather than native browser links.
- Desktop and mobile web; theme selection persists locally.

## Brand Commitments

The app name is servef. The interface follows VS Code conventions. No app logo.
