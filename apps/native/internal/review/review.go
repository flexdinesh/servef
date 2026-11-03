// Package review lays out one Source Control diff for native's diff viewer
// from the server's structured line diff (decision 0083), as the web UI's
// diff-content.ts and diff-metadata.ts do. Hunks, blocks, and gaps come
// from the server; native never compares texts or parses patches. It is
// pure.
package review

import (
	"fmt"
	"strings"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
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

// LineKind tells unchanged lines from deleted and added ones.
type LineKind uint8

const (
	Context LineKind = iota
	Deleted
	Added
)

// Line is one row of a hunk. Old and New are its 1-based lines in each
// side, 0 on the side it is not in. Text has no line ending.
type Line struct {
	Kind     LineKind
	Old, New int
	Text     string
}

// Hunk is a run of rows. Starts and counts are the server's: a side with
// no lines starts at the line that follows. Hidden counts the unchanged
// lines collapsed before it.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Hidden             int
	Lines              []Line
}

// Section is the hunk's caption: the unchanged lines collapsed before it.
func (h Hunk) Section() string {
	if h.Hidden == 0 {
		return ""
	}
	return Unmodified(h.Hidden)
}

// Unmodified reads "12 unmodified lines".
func Unmodified(n int) string {
	if n == 1 {
		return "1 unmodified line"
	}
	return fmt.Sprintf("%d unmodified lines", n)
}

// Layout is a diff laid out for the viewer.
type Layout struct {
	Kind Kind
	// Message explains a diff without hunks.
	Message string
	// Additions and Deletions are the server's counts.
	Additions, Deletions int
	Hunks                []Hunk
	// Hidden counts every collapsed unchanged line, 0 for a full file.
	Hidden int
	// Language is the server's highlight language, "text" for none.
	Language string
}

// Lay lays out diff. Full shows the whole file: every gap joins its
// neighbors into one hunk.
func Lay(diff contract.DiffResponse, full bool) Layout {
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
	old, new := splitLines(content(original)), splitLines(content(modified))
	gaps := map[int]contract.DiffGap{}
	for _, gap := range lineDiff.Gaps {
		gaps[gap.Hunk] = gap
	}
	for i, hunk := range lineDiff.Hunks {
		laid := Hunk{OldStart: hunk.OldStart, OldCount: hunk.OldCount, NewStart: hunk.NewStart, NewCount: hunk.NewCount, Hidden: gaps[i].Lines}
		for _, block := range hunk.Blocks {
			laid.Lines = appendBlock(laid.Lines, block, old, new)
		}
		layout.Hunks = append(layout.Hunks, laid)
		layout.Hidden += laid.Hidden
	}
	layout.Hidden += gaps[len(lineDiff.Hunks)].Lines
	if full {
		layout.Hunks, layout.Hidden = []Hunk{whole(layout.Hunks, gaps, old, new)}, 0
	}
	return layout
}

// whole joins hunks and the gaps around them into one hunk from line 1.
func whole(hunks []Hunk, gaps map[int]contract.DiffGap, old, new []string) Hunk {
	joined := Hunk{OldStart: 1, NewStart: 1}
	for i := 0; i <= len(hunks); i++ {
		if gap, ok := gaps[i]; ok {
			joined.Lines = appendBlock(joined.Lines, contract.DiffBlock{Kind: contract.DiffBlockContext, OldStart: gap.OldStart, OldCount: gap.Lines, NewStart: gap.NewStart, NewCount: gap.Lines}, old, new)
		}
		if i < len(hunks) {
			joined.Lines = append(joined.Lines, hunks[i].Lines...)
		}
	}
	for _, line := range joined.Lines {
		if line.Old > 0 {
			joined.OldCount++
		}
		if line.New > 0 {
			joined.NewCount++
		}
	}
	return joined
}

// appendBlock appends a block's rows: unchanged lines, or its deleted then
// added lines.
func appendBlock(lines []Line, block contract.DiffBlock, old, new []string) []Line {
	if block.Kind == contract.DiffBlockContext {
		for i := range block.OldCount {
			o, n := block.OldStart+i, block.NewStart+i
			text, ok := line(new, n)
			if !ok {
				text, _ = line(old, o)
			}
			lines = append(lines, Line{Kind: Context, Old: o, New: n, Text: text})
		}
		return lines
	}
	for i := range block.OldCount {
		text, _ := line(old, block.OldStart+i)
		lines = append(lines, Line{Kind: Deleted, Old: block.OldStart + i, Text: text})
	}
	for i := range block.NewCount {
		text, _ := line(new, block.NewStart+i)
		lines = append(lines, Line{Kind: Added, New: block.NewStart + i, Text: text})
	}
	return lines
}

// Sides lists, in order, the file line (1-based) of each row the viewer
// shows on the old side (unchanged and deleted rows) and on the new side
// (unchanged and added rows).
func (l Layout) Sides() (old, new []int) {
	for _, hunk := range l.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind != Added {
				old = append(old, line.Old)
			}
			if line.Kind != Deleted {
				new = append(new, line.New)
			}
		}
	}
	return old, new
}

// Rows picks, for each shown row of a side (Sides), the item of its file
// line in full, as each row's highlighted line: zero past full's end.
func Rows[T any](full []T, lines []int) []T {
	rows := make([]T, len(lines))
	for i, n := range lines {
		if n >= 1 && n <= len(full) {
			rows[i] = full[n-1]
		}
	}
	return rows
}

// line is the 1-based line n of lines.
func line(lines []string, n int) (string, bool) {
	if n < 1 || n > len(lines) {
		return "", false
	}
	return lines[n-1], true
}

// splitLines splits after each "\n", as the server does, and drops the
// line endings ("\r\n" too); empty text has no lines.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(strings.TrimSuffix(l, "\n"), "\r")
	}
	return lines
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
