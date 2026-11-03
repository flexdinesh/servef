package diff

import (
	"math"
	"time"
)

// component is a finished run of an edit path, linked to the run before it.
type component struct {
	kind     opKind
	count    int
	previous *component
}

// path is the furthest point an edit path reached on one diagonal; the
// zero path is no path. Its last run stays in the path (kind and count)
// until a run of another kind follows, so a search step allocates only when
// the run kind changes. Positions fit in int32: diffed sides are at most
// document.PreviewLimit bytes.
type path struct {
	runs   *component
	oldPos int32
	count  int32
	kind   opKind
	ok     bool
}

// finished is the path's runs with its last run closed.
func (p path) finished() *component {
	if p.count == 0 {
		return p.runs
	}
	return &component{kind: p.kind, count: int(p.count), previous: p.runs}
}

// myers is jsdiff's diff (Myers's O(ND) greedy search, with jsdiff's choice
// among equal-cost paths) over line ids, so hunks match what jsdiff shows
// for the same texts. It reports false past deadline.
func myers(oldIDs, newIDs []int32, deadline time.Time) ([]op, bool) {
	oldLen, newLen := len(oldIDs), len(newIDs)
	extractCommon := func(base *path, diagonal int) int {
		oldPos := int(base.oldPos)
		newPos := oldPos - diagonal
		common := 0
		for newPos+1 < newLen && oldPos+1 < oldLen && oldIDs[oldPos+1] == newIDs[newPos+1] {
			newPos++
			oldPos++
			common++
		}
		if common > 0 {
			base.runs, base.kind, base.count = base.finished(), equal, int32(common)
		}
		base.oldPos = int32(oldPos)
		return newPos
	}
	addToPath := func(from path, kind opKind, oldPosIncrement int32) path {
		if from.count > 0 && from.kind == kind {
			from.oldPos += oldPosIncrement
			from.count++
			return from
		}
		return path{oldPos: from.oldPos + oldPosIncrement, kind: kind, count: 1, runs: from.finished(), ok: true}
	}

	maxEditLength := oldLen + newLen
	// best holds diagonals -reach-1 through reach+1 and grows with the edit
	// length, which rarely nears its maximum.
	reach := min(maxEditLength, 64)
	offset := reach + 1
	best := make([]path, 2*reach+3)
	grow := func(editLength int) {
		if editLength <= reach {
			return
		}
		next := min(maxEditLength, max(editLength, 2*reach))
		grown := make([]path, 2*next+3)
		copy(grown[next-reach:], best)
		reach, offset, best = next, next+1, grown
	}
	best[offset] = path{oldPos: -1, ok: true}
	newPos := extractCommon(&best[offset], 0)
	if int(best[offset].oldPos)+1 >= oldLen && newPos+1 >= newLen {
		return buildOps(best[offset].finished()), true
	}
	// Once a path reaches the last old line, higher diagonals cannot do
	// better; once it reaches the last new line, lower ones cannot.
	minDiagonal, maxDiagonal := math.MinInt, math.MaxInt
	for editLength := 1; editLength <= maxEditLength; editLength++ {
		if time.Now().After(deadline) {
			return nil, false
		}
		grow(editLength)
		for diagonal := max(minDiagonal, -editLength); diagonal <= min(maxDiagonal, editLength); diagonal += 2 {
			removePath, addPath := best[offset+diagonal-1], best[offset+diagonal+1]
			if removePath.ok {
				best[offset+diagonal-1] = path{}
			}
			canAdd := false
			if addPath.ok {
				addedNewPos := int(addPath.oldPos) - diagonal
				canAdd = 0 <= addedNewPos && addedNewPos < newLen
			}
			canRemove := removePath.ok && int(removePath.oldPos)+1 < oldLen
			if !canAdd && !canRemove {
				best[offset+diagonal] = path{}
				continue
			}
			// Branch from the path furthest along the old lines.
			var base path
			if !canRemove || (canAdd && removePath.oldPos < addPath.oldPos) {
				base = addToPath(addPath, inserted, 0)
			} else {
				base = addToPath(removePath, deleted, 1)
			}
			newPos = extractCommon(&base, diagonal)
			if int(base.oldPos)+1 >= oldLen && newPos+1 >= newLen {
				return buildOps(base.finished()), true
			}
			best[offset+diagonal] = base
			if int(base.oldPos)+1 >= oldLen {
				maxDiagonal = min(maxDiagonal, diagonal-1)
			}
			if newPos+1 >= newLen {
				minDiagonal = max(minDiagonal, diagonal+1)
			}
		}
	}
	return nil, false
}

// buildOps turns a path's runs, last first, into an edit script.
func buildOps(last *component) []op {
	var reversed []op
	for run := last; run != nil; run = run.previous {
		reversed = append(reversed, op{kind: run.kind, lines: run.count})
	}
	var result script
	for i := len(reversed) - 1; i >= 0; i-- {
		result.add(reversed[i].kind, reversed[i].lines)
	}
	return result.ops
}
