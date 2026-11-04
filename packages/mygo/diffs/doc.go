// Package diffs is a diff viewer for mygo, a native port of what
// @pierre/diffs shows for a structured line diff: split and unified
// layouts, line numbers on both sides, unchanged gaps that collapse into
// separators and expand in place (up, down, all), word emphasis within
// changed line pairs, "No newline at end of file" marks, line selection
// with GitHub-style anchors, and hunk steps.
//
// It never compares texts into hunks: the host hands it a Diff whose
// hunks, blocks, and gaps someone else computed (a server, git), and the
// viewer lays out exactly those. Word emphasis compares the two lines of
// a changed pair only, as Pierre does, for presentation; it never moves a
// line into another hunk.
//
// The viewer draws its own rows in a virtualized ui.List rather than
// building on mygo's editor.DiffView: DiffView either diffs full texts
// itself or, through SetPatch, shows a patch's lines without the files,
// so it cannot show given hunks between collapsible gaps of the whole
// files, and its hunks are runs of changes rather than the given hunks
// that selections and steps address.
//
// Syntax colors come from the host through a span provider (State.Spans),
// so any highlighter fits. The package knows no transport or app.
package diffs
