---
version: 1
slug: "web-src-app-tsx"
primary_target: "web/src/App.tsx"
related_targets: []
---

# Markdown workspace

Mode: Operate for navigation; Read for Markdown preview.
Scope: existing workspace, explorer, preview, status bar, content search.
Constraints: local assets, one Go binary, strict TypeScript, existing diagrams/source/navigation.

## Direction contract

THESIS: A compact VS Code-style Markdown workspace. App identity and global actions move into the status bar.

OWN-WORLD: Neutral editor surfaces, flat rectangular rows, subtle borders, compact sans UI, proportional prose, monospace code. All named themes share semantic tokens.

STORY: Browse paths with Pierre, read a document, or search its workspace's content separately.

FIRST VIEWPORT: Workspace name above the left explorer; 32px tabs on the right; 15px prose with 28px H1 and 32px inset. Footer splits at the sidebar boundary: app name/state/count on the left; full relative file path, Find in files, size/theme on the right. Mobile uses a files drawer and 16px prose.

FORM: User-pinned VS Code conventions and explicitly approved proposal/wireframe; code-first implementation. No unchosen visual alternatives.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Unresolved decisions: none. Ctrl/Cmd-click adopts editor selection, as approved.
