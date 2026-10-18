# 0023: Git ignore sources

- Status: Accepted
- Date: 2026-10-18

## Context

Content search and navigation skip gitignored files, but 0015 limited the
rules to `.gitignore` files and `.git/info/exclude` in a root with a `.git`
directory. Global excludes (`~/.config/git/ignore`, `core.excludesFile`)
never applied, linked worktrees read no `info/exclude`, and a worktree's
`.git` file was indexed and searched. Search returned files git ignores.

## Decision

- When a project root is a repository or linked worktree, gitignore marking
  reads, in increasing precedence: `core.excludesFile`, the common git
  directory's `info/exclude`, then `.gitignore` files from the root down.
- `core.excludesFile` is the last value in `/etc/gitconfig` (skipped when
  `GIT_CONFIG_NOSYSTEM` is set), `$XDG_CONFIG_HOME/git/config`,
  `~/.gitconfig`, then the repository config; unset, it defaults to
  `$XDG_CONFIG_HOME/git/ignore` (`~/.config/git/ignore`). `~/` expands to
  home. Config includes are not followed.
- Linked worktrees resolve their `.git` file and `commondir` to the shared
  git directory. A `.git` file is always ignored.
- Outside a repository, only `.gitignore` files apply, as before.
- Ignored files stay browsable in the tree; content search and navigation
  locations skip them. servef reads git files directly and never runs `git`.

## Consequences

- Search matches `git status` for personal excludes such as editor or agent
  settings files.
- Supersedes the ignore-source bullet of 0015.
