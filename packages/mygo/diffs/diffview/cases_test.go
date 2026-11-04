package diffview

import "github.com/flexdinesh/servef/packages/mygo/diffs"

// lineDiffCases are two of the diffs package's cases (lineDiffOf in the
// web tests, generated in Node).
var lineDiffCases = map[string]struct {
	old, new string
	hunks    []diffs.Hunk
	gaps     []diffs.Gap
}{
	"long": {"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\ninserted\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", []diffs.Hunk{
		hunk(16, 8, 16, 9, block(diffs.Context, 16, 4, 16, 4), block(diffs.Change, 20, 0, 20, 1), block(diffs.Context, 20, 4, 21, 4))}, []diffs.Gap{gap(0, 1, 1, 15), gap(1, 24, 25, 17)}},
	"hunks": {"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nten\nTEN\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", []diffs.Hunk{
		hunk(6, 9, 6, 10, block(diffs.Context, 6, 4, 6, 4), block(diffs.Change, 10, 1, 10, 2), block(diffs.Context, 11, 4, 12, 4)),
		hunk(26, 9, 27, 8, block(diffs.Context, 26, 4, 27, 4), block(diffs.Change, 30, 1, 31, 0), block(diffs.Context, 31, 4, 31, 4))}, []diffs.Gap{gap(0, 1, 1, 5), gap(1, 15, 16, 11), gap(2, 35, 35, 6)}},
}

func diffOf(name string) *diffs.Diff {
	c := lineDiffCases[name]
	return &diffs.Diff{Old: diffs.SplitLines(c.old), New: diffs.SplitLines(c.new), Hunks: c.hunks, Gaps: c.gaps}
}

func sel(startSide diffs.Side, start int, endSide diffs.Side, end int) diffs.Selection {
	return diffs.Selection{Start: diffs.Point{Side: startSide, Line: start}, End: diffs.Point{Side: endSide, Line: end}}
}

func block(kind diffs.BlockKind, oldStart, oldCount, newStart, newCount int) diffs.Block {
	return diffs.Block{Kind: kind, OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}
}

func hunk(oldStart, oldCount, newStart, newCount int, blocks ...diffs.Block) diffs.Hunk {
	return diffs.Hunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount, Blocks: blocks}
}

func gap(h, oldStart, newStart, lines int) diffs.Gap {
	return diffs.Gap{Hunk: h, OldStart: oldStart, NewStart: newStart, Lines: lines}
}
