package diffs

import "strings"

// Side is a side of a diff: the old file or the new one.
type Side uint8

const (
	// Old is the older side (Pierre's deletions), L in anchors.
	Old Side = iota
	// New is the newer side (Pierre's additions), R in anchors.
	New
)

// Style lays a diff out side by side or in one column.
type Style uint8

const (
	// Split shows the old side on the left and the new on the right.
	Split Style = iota
	// Unified shows one column: a change's deleted lines, then its added
	// ones.
	Unified
)

// BlockKind tells unchanged lines from a change.
type BlockKind uint8

const (
	// Context is a run of unchanged lines: OldCount equals NewCount.
	Context BlockKind = iota
	// Change is OldCount deleted lines then NewCount added ones.
	Change
)

// Block is a run of a hunk's rows. Lines are numbered from 1; a side with
// no lines starts at the line that follows.
type Block struct {
	Kind               BlockKind
	OldStart, OldCount int
	NewStart, NewCount int
}

// Hunk is a run of changes with the unchanged lines around them.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Blocks             []Block
}

// Gap is a run of Lines unchanged lines collapsed before hunk Hunk, or
// after the last hunk when Hunk is the hunk count.
type Gap struct {
	Hunk               int
	OldStart, NewStart int
	Lines              int
}

// Diff is what the viewer shows: both whole sides and their structured
// line diff. Old and New are the files' lines split after each "\n",
// which each keeps (SplitLines); a missing side has none.
type Diff struct {
	Old, New []string
	Hunks    []Hunk
	Gaps     []Gap
}

// SplitLines splits text after each "\n", keeping it; empty text has no
// lines.
func SplitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Lines returns a side's lines.
func (d *Diff) Lines(side Side) []string {
	if side == Old {
		return d.Old
	}
	return d.New
}

// Text is line n (from 1) of a side without its line ending, "" past the
// side's ends.
func (d *Diff) Text(side Side, n int) string {
	lines := d.Lines(side)
	if n < 1 || n > len(lines) {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(lines[n-1], "\n"), "\r")
}

// gap returns the gap before hunk i, or after the last hunk when i is the
// hunk count.
func (d *Diff) gap(i int) (Gap, bool) {
	for _, g := range d.Gaps {
		if g.Hunk == i && g.Lines > 0 {
			return g, true
		}
	}
	return Gap{}, false
}

// endsWithoutNewline reports a hunk side that reaches its file's last
// line, which lacks a newline.
func (d *Diff) endsWithoutNewline(side Side, start, count int) bool {
	lines := d.Lines(side)
	return count > 0 && start+count-1 == len(lines) && !strings.HasSuffix(lines[len(lines)-1], "\n")
}
