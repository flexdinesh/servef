# 0052: Welcome tab and an empty pane that follows the sidebar view

- Status: Accepted
- Date: 2026-10-27
- Supersedes parts of: [0007](0007-projects-and-project-picker.md) (the
  picker opening on first landing)

## Context

A project with no remembered tabs opened on an empty pane reading “Choose a
file: Select a file from the folder tree.”, whatever the sidebar showed, so
Search, Source Control, and Walkthroughs pointed at a tree that was not
there. A first landing with several projects opened the project picker over
that pane. Nothing in the app said what servef does beyond its views, and the
`servef` footer brand only reopened the last tab.

## Decision

### Welcome tab

- A tab kind, `{ kind: "welcome" }`, at most one, labelled “Welcome” with a
  house icon. Route `/view?project=…&welcome=1`, without a path, review, or
  walkthrough; it loads only the tree. Stored as `{ welcome: true }`, it
  restores like any tab.
- It opens on its own when a project's route names no tab and memory restores
  no tabs, while **Workbench › Show Welcome on Startup** (`showWelcome`, On by
  default, in `servef-settings`) is On. Restored tabs without an active one
  keep the empty pane. Projects mount separately, so this holds per project.
- The footer brand, “Help › Welcome” in the command palette, and the empty
  pane's “Show Welcome” open or activate it.
- Content, on the document surface without cards: a muted “Serving <roots>”
  line, “servef” at the preview h1 size, and a one-line tagline; then two
  columns once the tab is 48rem wide (a container query), one below.
  - **Start**: Open project… (Cmd/Ctrl+O), Find a file (Cmd/Ctrl+K), Show all
    commands (Cmd/Ctrl+Shift+P).
  - **This project**: the branch, the uncommitted change count (opening the
    Uncommitted review), and new or listed walkthroughs (opening the view).
    Counts not yet read are left out; a folder says it is not a repository.
  - **Projects**, only with more than one available project: the picker's
    sections (`projectSections()`) without Browse, a filter field that
    flattens them into the picker's ranked matches, repositories with several
    worktrees collapsed under a chevron, the current project marked, and
    loading, error with Retry, and no-match states.
  - **What you can do**: Find in files, Review changes, Follow walkthroughs,
    Go to definition (opens Settings › Language Servers), Change the color
    theme, and Adjust the font size, each a link that runs it, with a one-line
    description and its shortcut; then a Documentation link to the GitHub
    README in a new tab.
  - A footer line, “Shown when a project opens with no tabs. Change in
    Settings.”, opening the setting.
- While Welcome is the active tab, the change list rereads on return to the
  window as it does for an open Source Control view.

### Landing

- With Show Welcome on, a first landing with several projects no longer opens
  the picker: Welcome lists them. With it off, the picker opens as before.

### Empty pane

- With no tab open, the pane follows the sidebar's view (phones: the last
  active view), from pure `emptyWorkspace()`:

  | View | State | Heading | Action |
  | --- | --- | --- | --- |
  | Explorer | | Choose a file | Find a file |
  | Search | no results | Search file contents | |
  | Search | results | Open a match | |
  | Source Control | changes | Review your changes | Review uncommitted changes |
  | Source Control | clean | Review history | |
  | Source Control | folder | Not a git repository | |
  | Walkthroughs | listed or unread | Choose a walkthrough | |
  | Walkthroughs | none | No walkthroughs yet | |
  | Hidden sidebar | | Nothing open | Show sidebar |

- On phones a view with no action offers “Open <view>”, which opens its
  drawer. Every variant ends with a “Show Welcome” link.

## Consequences

- The project list exists in the picker and in Welcome; both read
  `projectSections()`, so grouping and ranking stay identical.
- Welcome rereads `/api/projects`, which rediscovers on every call, once per
  mount.
- The empty pane reads Search, Source Control, and Walkthroughs state the
  workspace already holds; it adds no requests.
