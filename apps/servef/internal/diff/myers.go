package diff

import (
	"math"
	"time"
)

// component is a run of an edit path, linked to the run before it.
type component struct {
	kind     opKind
	count    int
	previous *component
}

// path is the furthest point an edit path reached on one diagonal.
type path struct {
	oldPos int
	last   *component
}

// myers is jsdiff's diff (Myers's O(ND) greedy search, with jsdiff's choice
// among equal-cost paths) over line ids, so hunks match what jsdiff shows
// for the same texts. It reports false past deadline.
func myers(oldIDs, newIDs []int32, deadline time.Time) ([]op, bool) {
	oldLen, newLen := len(oldIDs), len(newIDs)
	extractCommon := func(base *path, diagonal int) int {
		oldPos := base.oldPos
		newPos := oldPos - diagonal
		common := 0
		for newPos+1 < newLen && oldPos+1 < oldLen && oldIDs[oldPos+1] == newIDs[newPos+1] {
			newPos++
			oldPos++
			common++
		}
		if common > 0 {
			base.last = &component{kind: equal, count: common, previous: base.last}
		}
		base.oldPos = oldPos
		return newPos
	}
	addToPath := func(from *path, kind opKind, oldPosIncrement int) *path {
		last := from.last
		if last != nil && last.kind == kind {
			return &path{oldPos: from.oldPos + oldPosIncrement, last: &component{kind: kind, count: last.count + 1, previous: last.previous}}
		}
		return &path{oldPos: from.oldPos + oldPosIncrement, last: &component{kind: kind, count: 1, previous: last}}
	}

	maxEditLength := oldLen + newLen
	offset := maxEditLength + 1
	best := make([]*path, 2*maxEditLength+3)
	best[offset] = &path{oldPos: -1}
	newPos := extractCommon(best[offset], 0)
	if best[offset].oldPos+1 >= oldLen && newPos+1 >= newLen {
		return buildOps(best[offset].last), true
	}
	// Once a path reaches the last old line, higher diagonals cannot do
	// better; once it reaches the last new line, lower ones cannot.
	minDiagonal, maxDiagonal := math.MinInt, math.MaxInt
	for editLength := 1; editLength <= maxEditLength; editLength++ {
		if time.Now().After(deadline) {
			return nil, false
		}
		for diagonal := max(minDiagonal, -editLength); diagonal <= min(maxDiagonal, editLength); diagonal += 2 {
			removePath, addPath := best[offset+diagonal-1], best[offset+diagonal+1]
			if removePath != nil {
				best[offset+diagonal-1] = nil
			}
			canAdd := false
			if addPath != nil {
				addedNewPos := addPath.oldPos - diagonal
				canAdd = 0 <= addedNewPos && addedNewPos < newLen
			}
			canRemove := removePath != nil && removePath.oldPos+1 < oldLen
			if !canAdd && !canRemove {
				best[offset+diagonal] = nil
				continue
			}
			// Branch from the path furthest along the old lines.
			var base *path
			if !canRemove || (canAdd && removePath.oldPos < addPath.oldPos) {
				base = addToPath(addPath, inserted, 0)
			} else {
				base = addToPath(removePath, deleted, 1)
			}
			newPos = extractCommon(base, diagonal)
			if base.oldPos+1 >= oldLen && newPos+1 >= newLen {
				return buildOps(base.last), true
			}
			best[offset+diagonal] = base
			if base.oldPos+1 >= oldLen {
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
