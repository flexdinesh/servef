// Package review lays out one Source Control diff for native's diff viewer
// (packages/mygo/diffs) from the server's structured line diff (decision
// 0083), as the web UI's diff-metadata.ts lays it out for Pierre. Hunks,
// blocks, and gaps come from the server; native never compares texts or
// parses patches. It is pure: package diffs imports no mygo.
package review

import (
	"fmt"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/packages/mygo/diffs"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Kind is what a diff can show.
type Kind uint8

const (
	// Text has hunks to show.
	Text Kind = iota
	// Unchanged has text sides and no hunks.
	Unchanged
	// Binary has a binary side.
	Binary
	// TooLarge has a side over the 0004 limits.
	TooLarge
	// Unavailable has text sides but no line diff.
	Unavailable
)

// Layout is a diff laid out for the viewer.
type Layout struct {
	Kind Kind
	// Message explains a diff without hunks.
	Message string
	// Additions and Deletions are the server's counts.
	Additions, Deletions int
	// Diff is the server's line diff over both whole sides, as the
	// viewer takes it; nil unless Kind is Text.
	Diff *diffs.Diff
	// Language is the server's highlight language, "text" for none.
	Language string
}

// Lay lays out diff: its hunks, blocks, and gaps exactly as the server
// sent them, over both whole sides.
func Lay(diff contract.DiffResponse) Layout {
	language := diff.DocumentType.Language
	if language == "" {
		language = "text"
	}
	original, modified := diff.Original, diff.Modified
	switch {
	case binary(original) || binary(modified):
		return Layout{Kind: Binary, Message: binaryLabel(original, modified), Language: language}
	case tooLarge(original) || tooLarge(modified):
		return Layout{Kind: TooLarge, Message: fmt.Sprintf("Large file (%s): diff is off.", docview.Bytes(max(size(original), size(modified)))), Language: language}
	case diff.LineDiff == nil:
		return Layout{Kind: Unavailable, Message: "Couldn't compute this diff.", Language: language}
	}
	lineDiff := *diff.LineDiff
	layout := Layout{Additions: lineDiff.Additions, Deletions: lineDiff.Deletions, Language: language}
	if len(lineDiff.Hunks) == 0 {
		layout.Kind, layout.Message = Unchanged, "File contents are unchanged."
		if lineDiff.IgnoreWhitespace {
			layout.Message = "No changes apart from whitespace."
		}
		return layout
	}
	d := &diffs.Diff{Old: diffs.SplitLines(content(original)), New: diffs.SplitLines(content(modified))}
	for _, hunk := range lineDiff.Hunks {
		laid := diffs.Hunk{OldStart: hunk.OldStart, OldCount: hunk.OldCount, NewStart: hunk.NewStart, NewCount: hunk.NewCount}
		for _, block := range hunk.Blocks {
			kind := diffs.Context
			if block.Kind == contract.DiffBlockChange {
				kind = diffs.Change
			}
			laid.Blocks = append(laid.Blocks, diffs.Block{Kind: kind, OldStart: block.OldStart, OldCount: block.OldCount, NewStart: block.NewStart, NewCount: block.NewCount})
		}
		d.Hunks = append(d.Hunks, laid)
	}
	for _, gap := range lineDiff.Gaps {
		d.Gaps = append(d.Gaps, diffs.Gap{Hunk: gap.Hunk, OldStart: gap.OldStart, NewStart: gap.NewStart, Lines: gap.Lines})
	}
	layout.Diff = d
	return layout
}

func content(side *contract.DiffSide) string {
	if side == nil || side.Content == nil {
		return ""
	}
	return *side.Content
}

func binary(side *contract.DiffSide) bool   { return side != nil && side.Binary }
func tooLarge(side *contract.DiffSide) bool { return side != nil && side.TooLarge }

func size(side *contract.DiffSide) int64 {
	if side == nil {
		return 0
	}
	return side.Size
}

// binaryLabel reads "Binary file changed · 12.0 KB → 14.0 KB", as the web
// UI's binaryChangeLabel.
func binaryLabel(original, modified *contract.DiffSide) string {
	switch {
	case original != nil && modified != nil:
		return fmt.Sprintf("Binary file changed · %s → %s", docview.Bytes(original.Size), docview.Bytes(modified.Size))
	case modified != nil:
		return "Binary file added · " + docview.Bytes(modified.Size)
	}
	return "Binary file deleted · " + docview.Bytes(size(original))
}

// Letter is a change status's letter, as the web UI's Source Control.
func Letter(status contract.ChangeStatus) string {
	switch status {
	case contract.ChangeModified:
		return "M"
	case contract.ChangeAdded:
		return "A"
	case contract.ChangeUntracked:
		return "U"
	case contract.ChangeDeleted:
		return "D"
	case contract.ChangeRenamed:
		return "R"
	}
	return "?"
}

// StatusName is a change status's name.
func StatusName(status contract.ChangeStatus) string {
	switch status {
	case contract.ChangeModified:
		return "Modified"
	case contract.ChangeAdded:
		return "Added"
	case contract.ChangeUntracked:
		return "Untracked"
	case contract.ChangeDeleted:
		return "Deleted"
	case contract.ChangeRenamed:
		return "Renamed"
	}
	return string(status)
}

// Label names a changed file: "old → new" for a rename.
func Label(file contract.ChangedFile) string {
	if file.Status == contract.ChangeRenamed && file.OldPath != "" {
		return file.OldPath + " → " + file.Path
	}
	return file.Path
}

// ChangeCount reads "3 changes".
func ChangeCount(n int) string {
	if n == 1 {
		return "1 change"
	}
	return fmt.Sprintf("%d changes", n)
}

// Counts reads "+3 −1".
func Counts(additions, deletions int) string {
	return fmt.Sprintf("+%d −%d", additions, deletions)
}
