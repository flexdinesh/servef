// Package diff computes line diffs (decision 0083): the edit script between
// two texts, its hunks with context, the unchanged gaps between them, and
// line counts. It knows text only; no git, files, or transport.
//
// Lines end after "\n", which stays part of the line, so a last line without
// one differs from the same text with one. Hunks follow jsdiff's unified
// patch layout, which the browser's diff view (Pierre) used before diffs
// moved to the server: four lines of context, hunks joined when their context
// overlaps, and change blocks re-paired by line similarity.
package diff

import (
	"bytes"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	// Context is how many unchanged lines a hunk shows on each side of its
	// changes.
	Context = 4

	// lineDiffTimeout bounds one line diff; past it the remaining span counts
	// as removed and added, a correct but larger edit.
	lineDiffTimeout = time.Second
	// surrogates are the runes a line id skips: UTF-16 surrogate halves are
	// not valid runes.
	surrogateStart = 0xD800
	surrogateCount = 0x800
	// lineIDs is how many distinct lines a diff can tell apart.
	lineIDs = utf8.MaxRune + 1 - surrogateCount
)

// Diff bounds; variables so tests can lower them.
var (
	// maxChanges bounds the change runs one diff reports. A diff with more
	// reports its differing span as one change instead: a correct but larger
	// edit with a bounded response.
	maxChanges = 1 << 14
	// myersTimeout bounds the edit graph search of one diff; past it the diff
	// falls back to the bisecting diff, which is linear in memory.
	myersTimeout = 250 * time.Millisecond
)

// Options choose how lines compare.
type Options struct {
	// IgnoreWhitespace compares lines without their leading and trailing
	// whitespace, as JavaScript's String.prototype.trim removes it.
	IgnoreWhitespace bool
}

// Block is a run of rows within a hunk: unchanged lines (OldCount equals
// NewCount), or one change of OldCount deleted lines then NewCount added
// ones. Starts are 1-based line numbers; a side with no lines starts at the
// line that follows the block.
type Block struct {
	Change             bool
	OldStart, OldCount int
	NewStart, NewCount int
}

// Hunk is a run of changes with up to Context unchanged lines around each.
// Starts follow Block's.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	// Additions and Deletions count the hunk's changed lines.
	Additions, Deletions int
	Blocks               []Block
}

// Gap is a run of unchanged lines between hunks that a hunk view collapses:
// before hunk Hunk, or after the last hunk when Hunk is the hunk count.
type Gap struct {
	Hunk     int
	OldStart int
	NewStart int
	Lines    int
}

// Result is a line diff. A diff with no hunks has no changes (or, ignoring
// whitespace, whitespace changes only) and no gaps.
type Result struct {
	Hunks []Hunk
	Gaps  []Gap
	// Additions and Deletions count every hunk's changed lines.
	Additions, Deletions int
}

// op is one run of an edit script.
type op struct {
	kind  opKind
	lines int
}

type opKind uint8

const (
	equal opKind = iota
	deleted
	inserted
)

// Lines diffs old against new.
func Lines(old, new []byte, options Options) Result {
	oldLines, newLines := splitLines(old), splitLines(new)
	oldIDs, newIDs := lineIDsOf(oldLines, newLines, options)
	ops, ok := myers(oldIDs, newIDs, time.Now().Add(myersTimeout))
	if !ok {
		ops = editScript(oldLines, newLines, options)
	}
	if changes(ops) > maxChanges {
		ops = coarsen(ops)
	}
	result := Result{Hunks: hunks(ops)}
	for i := range result.Hunks {
		hunk := &result.Hunks[i]
		hunk.Blocks = realign(hunk.Blocks, oldLines, newLines)
		result.Additions += hunk.Additions
		result.Deletions += hunk.Deletions
	}
	result.Gaps = gaps(result.Hunks, len(oldLines), len(newLines))
	return result
}

// Count counts the lines a line diff from old to new adds and removes, as
// git diff --numstat does.
func Count(old, new []byte) (additions, deletions int) {
	for _, run := range editScript(splitLines(old), splitLines(new), Options{}) {
		switch run.kind {
		case inserted:
			additions += run.lines
		case deleted:
			deletions += run.lines
		case equal:
		}
	}
	return additions, deletions
}

// lineKey is what a line compares as.
func lineKey(options Options) func([]byte) []byte {
	if options.IgnoreWhitespace {
		return trimSpace
	}
	return func(line []byte) []byte { return line }
}

// lineIDsOf numbers each side's lines so equal lines share an id.
func lineIDsOf(oldLines, newLines [][]byte, options Options) ([]int32, []int32) {
	key := lineKey(options)
	ids := make(map[string]int32, len(oldLines))
	number := func(lines [][]byte) []int32 {
		result := make([]int32, len(lines))
		for i, line := range lines {
			id, ok := ids[string(key(line))]
			if !ok {
				id = int32(len(ids))
				ids[string(key(line))] = id
			}
			result[i] = id
		}
		return result
	}
	return number(oldLines), number(newLines)
}

// editScript is the line edit script from oldLines to newLines by
// bisection with a time bound, trimming the common prefix and suffix first:
// equal, deleted, and inserted runs, never two of a kind in a row,
// deletions before insertions between equal runs.
func editScript(oldLines, newLines [][]byte, options Options) []op {
	key := lineKey(options)
	same := func(a, b []byte) bool { return bytes.Equal(key(a), key(b)) }
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && same(oldLines[prefix], newLines[prefix]) {
		prefix++
	}
	oldMiddle, newMiddle := oldLines[prefix:], newLines[prefix:]
	suffix := 0
	for suffix < len(oldMiddle) && suffix < len(newMiddle) && same(oldMiddle[len(oldMiddle)-1-suffix], newMiddle[len(newMiddle)-1-suffix]) {
		suffix++
	}
	oldMiddle, newMiddle = oldMiddle[:len(oldMiddle)-suffix], newMiddle[:len(newMiddle)-suffix]

	var script script
	script.add(equal, prefix)
	script.middle(oldMiddle, newMiddle, key)
	script.add(equal, suffix)
	return script.ops
}

// script builds an edit script, merging runs of a kind.
type script struct{ ops []op }

func (s *script) add(kind opKind, lines int) {
	if lines == 0 {
		return
	}
	if last := len(s.ops) - 1; last >= 0 && s.ops[last].kind == kind {
		s.ops[last].lines += lines
		return
	}
	s.ops = append(s.ops, op{kind: kind, lines: lines})
}

// middle diffs the span between the common prefix and suffix.
func (s *script) middle(oldLines, newLines [][]byte, key func([]byte) []byte) {
	if len(oldLines) == 0 || len(newLines) == 0 {
		s.add(deleted, len(oldLines))
		s.add(inserted, len(newLines))
		return
	}
	// Each distinct line becomes one rune, so a rune diff is a line diff.
	ids := map[string]rune{}
	encode := func(lines [][]byte) ([]rune, bool) {
		runes := make([]rune, len(lines))
		for i, line := range lines {
			id, ok := ids[string(key(line))]
			if !ok {
				if len(ids) == lineIDs {
					return nil, false
				}
				id = rune(len(ids))
				if id >= surrogateStart {
					id += surrogateCount
				}
				ids[string(key(line))] = id
			}
			runes[i] = id
		}
		return runes, true
	}
	oldRunes, oldOK := encode(oldLines)
	newRunes, newOK := encode(newLines)
	if !oldOK || !newOK {
		s.add(deleted, len(oldLines))
		s.add(inserted, len(newLines))
		return
	}
	differ := diffmatchpatch.New()
	differ.DiffTimeout = lineDiffTimeout
	// Runs between equal ones list deletions first.
	pendingDeleted, pendingInserted := 0, 0
	flush := func() {
		s.add(deleted, pendingDeleted)
		s.add(inserted, pendingInserted)
		pendingDeleted, pendingInserted = 0, 0
	}
	for _, run := range differ.DiffMainRunes(oldRunes, newRunes, false) {
		lines := utf8.RuneCountInString(run.Text)
		switch run.Type {
		case diffmatchpatch.DiffDelete:
			pendingDeleted += lines
		case diffmatchpatch.DiffInsert:
			pendingInserted += lines
		case diffmatchpatch.DiffEqual:
			flush()
			s.add(equal, lines)
		}
	}
	flush()
}

// changes counts an edit script's change runs: deletions, insertions, or
// both between equal runs.
func changes(ops []op) int {
	count := 0
	for i, run := range ops {
		if run.kind != equal && (i == 0 || ops[i-1].kind == equal) {
			count++
		}
	}
	return count
}

// coarsen keeps an edit script's leading and trailing equal runs and makes
// everything between them one change.
func coarsen(ops []op) []op {
	var result script
	start, end := 0, len(ops)
	if start < end && ops[start].kind == equal {
		result.add(equal, ops[start].lines)
		start++
	}
	trailing := 0
	if end > start && ops[end-1].kind == equal {
		trailing = ops[end-1].lines
		end--
	}
	removed, added := 0, 0
	for _, run := range ops[start:end] {
		switch run.kind {
		case equal:
			removed += run.lines
			added += run.lines
		case deleted:
			removed += run.lines
		case inserted:
			added += run.lines
		}
	}
	result.add(deleted, removed)
	result.add(inserted, added)
	result.add(equal, trailing)
	return result.ops
}

// hunks groups an edit script into hunks as jsdiff's structuredPatch does:
// each change opens a hunk with up to Context lines of the equal run before
// it; an equal run of at most 2*Context lines that is not the last run joins
// the hunk; a longer one, or the last, closes it after Context lines.
func hunks(ops []op) []Hunk {
	var result []Hunk
	var open *Hunk
	oldLine, newLine := 1, 1
	// context adds lines unchanged lines from oldStart and newStart.
	context := func(oldStart, newStart, lines int) {
		if lines > 0 {
			open.Blocks = append(open.Blocks, Block{OldStart: oldStart, OldCount: lines, NewStart: newStart, NewCount: lines})
		}
	}
	for i, run := range ops {
		if run.kind == equal {
			if open != nil {
				if run.lines <= 2*Context && i < len(ops)-1 {
					context(oldLine, newLine, run.lines)
				} else {
					context(oldLine, newLine, min(run.lines, Context))
					open.OldCount = oldLine + min(run.lines, Context) - open.OldStart
					open.NewCount = newLine + min(run.lines, Context) - open.NewStart
					result = append(result, *open)
					open = nil
				}
			}
			oldLine += run.lines
			newLine += run.lines
			continue
		}
		if open == nil {
			lead := 0
			if i > 0 {
				lead = min(ops[i-1].lines, Context)
			}
			open = &Hunk{OldStart: oldLine - lead, NewStart: newLine - lead}
			context(open.OldStart, open.NewStart, lead)
		}
		// A change is its deletions then its insertions, one block.
		if last := len(open.Blocks) - 1; last < 0 || !open.Blocks[last].Change {
			open.Blocks = append(open.Blocks, Block{Change: true, OldStart: oldLine, NewStart: newLine})
		}
		block := &open.Blocks[len(open.Blocks)-1]
		if run.kind == deleted {
			block.OldCount += run.lines
			open.Deletions += run.lines
			oldLine += run.lines
		} else {
			block.NewCount += run.lines
			open.Additions += run.lines
			newLine += run.lines
		}
	}
	if open != nil {
		open.OldCount = oldLine - open.OldStart
		open.NewCount = newLine - open.NewStart
		result = append(result, *open)
	}
	return result
}

// gaps lists the unchanged runs before each hunk and after the last; a diff
// without hunks has none.
func gaps(hunks []Hunk, oldLines, newLines int) []Gap {
	if len(hunks) == 0 {
		return nil
	}
	var result []Gap
	oldLine, newLine := 1, 1
	add := func(index, lines int) {
		if lines > 0 {
			result = append(result, Gap{Hunk: index, OldStart: oldLine, NewStart: newLine, Lines: lines})
		}
	}
	for i, hunk := range hunks {
		add(i, hunk.NewStart-newLine)
		oldLine, newLine = hunk.OldStart+hunk.OldCount, hunk.NewStart+hunk.NewCount
	}
	add(len(hunks), min(oldLines+1-oldLine, newLines+1-newLine))
	return result
}

// splitLines splits content after each "\n", keeping it; a last line
// without one is kept as it is.
func splitLines(content []byte) [][]byte {
	lines := make([][]byte, 0, bytes.Count(content, []byte{'\n'})+1)
	for len(content) > 0 {
		end := bytes.IndexByte(content, '\n') + 1
		if end == 0 {
			end = len(content)
		}
		lines = append(lines, content[:end])
		content = content[end:]
	}
	return lines
}

// trimSpace removes leading and trailing JavaScript whitespace.
func trimSpace(line []byte) []byte {
	return bytes.TrimFunc(line, isJSSpace)
}

// isJSSpace reports JavaScript's WhiteSpace and LineTerminator characters,
// which String.prototype.trim and the regular expression \s match.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}
